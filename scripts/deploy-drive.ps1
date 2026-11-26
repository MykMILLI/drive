<#
.SYNOPSIS
  One-command deploy for Drive: commit -> push to GitHub -> package -> scp -> build on the host -> swap container.

.DESCRIPTION
  What it does, in order:
    1. Checks you are on master, runs quick checks (go vet, typecheck), and commits any local changes
       (only if you pass -Message).
    2. Makes sure GitHub has exactly what will be deployed (git push, then compares HEAD to origin/master).
    3. Packages the committed code with `git archive` and copies it to the host with scp.
    4. On the host: backs up the database, builds the image from Dockerfile.deploy, stops the running
       container (kept as <name>-prev-<time>), starts the new one with the SAME network/port/volumes/env,
       waits for it to be healthy, and rolls back by itself if it is not.

.EXAMPLES
  .\deploy-drive.ps1 -Message "feat: projects dashboard tweaks"   # commit + push + deploy
  .\deploy-drive.ps1                                              # deploy what is already committed
  .\deploy-drive.ps1 -Rollback                                    # put the previous container back
  .\deploy-drive.ps1 -DryRun                                      # show the plan only
#>
[CmdletBinding()]
param(
    [string]$Message = '',
    [string]$RepoPath = 'C:\clones\drive',
    [string]$SshTarget = 'millitop-host',
    [string]$Container = 'millitop-drive',
    [string]$Branch = 'master',
    [switch]$SkipChecks,
    [switch]$Rollback,
    [switch]$DryRun
)

$ErrorActionPreference = 'Stop'
$utf8NoBom = New-Object System.Text.UTF8Encoding($false)
function Step($m) { Write-Host "== $m" -ForegroundColor Cyan }
function Run([string]$exe, [string[]]$Arguments) {
    & $exe @Arguments
    if ($LASTEXITCODE -ne 0) { throw "$exe $($Arguments -join ' ') failed (exit $LASTEXITCODE)" }
}
function GitOut([string[]]$Arguments) {
    $out = & git @Arguments
    if ($LASTEXITCODE -ne 0) { throw "git $($Arguments -join ' ') failed (exit $LASTEXITCODE)" }
    return $out
}

$remote = @'
#!/usr/bin/env bash
# Runs on the host. Usage:
#   remote-deploy.sh deploy   <container> <short-sha> <source.tar.gz>
#   remote-deploy.sh rollback <container>
set -euo pipefail
MODE="$1"; C="$2"; SHA="${3:-}"; TGZ="${4:-}"
BUILD_ROOT="$HOME/custom-apps/drive-build"
BACKUP_ROOT="$HOME/drive-backups"
BASE_IMAGE="drive-runtime:original"

rollback_to() {
  local prev="$1"
  echo "== rolling back to $prev"
  docker stop "$C" >/dev/null 2>&1 || true
  docker rm "$C" >/dev/null 2>&1 || true
  docker rename "$prev" "$C"
  docker start "$C" >/dev/null
  docker ps --filter "name=^${C}\$" --format 'table {{.Names}}\t{{.Image}}\t{{.Status}}'
}

if [ "$MODE" = "rollback" ]; then
  prev="$(docker ps -a --filter "name=^${C}-prev-" --format '{{.Names}}' | sort -r | head -1)"
  [ -n "$prev" ] || { echo "No ${C}-prev-* container found to roll back to." >&2; exit 1; }
  rollback_to "$prev"
  exit 0
fi

TS="$(date +%Y%m%d-%H%M%S)"
docker image inspect "$BASE_IMAGE" >/dev/null || { echo "Missing base image $BASE_IMAGE" >&2; exit 1; }
docker inspect "$C" >/dev/null || { echo "Container $C not found" >&2; exit 1; }

DIR="$BUILD_ROOT/$TS-$SHA"
mkdir -p "$DIR" "$BACKUP_ROOT"
tar xzf "$TGZ" -C "$DIR"
[ -f "$DIR/Dockerfile.deploy" ] || { echo "Package is missing Dockerfile.deploy" >&2; exit 1; }
echo "== unpacked $SHA into $DIR"

echo "== reading settings from the running container"
NETWORK="$(docker inspect -f '{{.HostConfig.NetworkMode}}' "$C")"
RESTART="$(docker inspect -f '{{.HostConfig.RestartPolicy.Name}}' "$C")"
if [ -z "$RESTART" ] || [ "$RESTART" = "no" ]; then RESTART="unless-stopped"; fi
HOSTPORT="$(docker port "$C" 80/tcp | head -1 | sed 's/.*://')"
mapfile -t ENVS < <(docker inspect -f '{{range .Config.Env}}{{println .}}{{end}}' "$C" | grep -E '^(PUID|PGID|TZ|FB_|DRIVE_)' || true)
mapfile -t MOUNTS < <(docker inspect -f '{{range .Mounts}}{{if eq .Type "volume"}}{{.Name}}{{else}}{{.Source}}{{end}}:{{.Destination}}{{if not .RW}}:ro{{end}}{{println}}{{end}}' "$C" | sed '/^$/d')
DBMOUNT="$(docker inspect -f '{{range .Mounts}}{{if eq .Destination "/database"}}{{if eq .Type "volume"}}{{.Name}}{{else}}{{.Source}}{{end}}{{end}}{{end}}' "$C")"
echo "   network=$NETWORK port=$HOSTPORT restart=$RESTART mounts=${#MOUNTS[@]} env=${#ENVS[@]}"
[ -n "$HOSTPORT" ] && [ "${#MOUNTS[@]}" -gt 0 ] || { echo "Could not read port/mounts - aborting before any change" >&2; exit 1; }

if [ -n "$DBMOUNT" ]; then
  BK="$BACKUP_ROOT/db-$TS"; mkdir -p "$BK"
  docker run --rm --entrypoint cp -v "$DBMOUNT":/src:ro -v "$BK":/dst "$BASE_IMAGE" -a /src/. /dst/
  echo "== database backed up to $BK"
fi

NEWIMG="millitop-drive:$TS-$SHA"
echo "== building $NEWIMG (a few minutes)"
( cd "$DIR" && docker build -f Dockerfile.deploy -t "$NEWIMG" . )

ARGS=(run -d --name "$C" --restart "$RESTART" --network "$NETWORK" -p "$HOSTPORT:80")
for e in "${ENVS[@]}"; do ARGS+=(-e "$e"); done
for m in "${MOUNTS[@]}"; do ARGS+=(-v "$m"); done

PREV="$C-prev-$TS"
echo "== swapping containers (old one kept as $PREV)"
docker stop "$C" >/dev/null
docker rename "$C" "$PREV"
if ! docker "${ARGS[@]}" "$NEWIMG" >/dev/null; then
  echo "docker run failed" >&2
  rollback_to "$PREV"
  exit 1
fi

echo "== waiting for the new container to be healthy"
ok=0
for i in $(seq 1 30); do
  sleep 2
  state="$(docker inspect -f '{{.State.Status}}' "$C" 2>/dev/null || echo gone)"
  [ "$state" = "running" ] || break
  health="$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' "$C" 2>/dev/null || echo none)"
  if [ "$health" = "healthy" ] || { [ "$health" = "none" ] && [ "$i" -ge 8 ]; }; then ok=1; break; fi
done
if [ "$ok" -ne 1 ]; then
  echo "New container did not become healthy. Last log lines:" >&2
  docker logs --tail 40 "$C" 2>&1 || true
  rollback_to "$PREV"
  exit 1
fi

code="$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:$HOSTPORT/api/projects" || true)"
if [ "$code" = "401" ]; then echo "== OK: Projects routes are live (/api/projects -> 401)"
else echo "== container healthy; /api/projects returned '$code' (401 expected unless served under a base URL)"; fi

docker ps -a --filter "name=^${C}-prev-" --format '{{.Names}}' | sort -r | tail -n +4 | xargs -r docker rm >/dev/null || true
ls -1dt "$BUILD_ROOT"/*/ 2>/dev/null | tail -n +4 | xargs -r rm -rf
rm -f "$TGZ"
echo "$SHA $TS $NEWIMG" > "$BUILD_ROOT/DEPLOYED.txt"
echo "== deployed $SHA as $NEWIMG. Rollback: .\deploy-drive.ps1 -Rollback"
'@

$RepoPath = (Resolve-Path -LiteralPath $RepoPath).Path
foreach ($tool in 'git', 'ssh', 'scp') {
    if (-not (Get-Command $tool -ErrorAction SilentlyContinue)) { throw "$tool is not on PATH" }
}
Push-Location $RepoPath
try {
    if (-not (Test-Path 'go.mod')) { throw "go.mod not found in $RepoPath" }

    $work = Join-Path $env:TEMP 'drive-deploy'
    New-Item -ItemType Directory -Force -Path $work | Out-Null
    $remoteScript = Join-Path $work 'drive-remote-deploy.sh'
    [IO.File]::WriteAllText($remoteScript, ($remote -replace "`r`n", "`n"), $utf8NoBom)

    if ($Rollback) {
        Step "Rolling back $Container on $SshTarget"
        if ($DryRun) { Write-Host 'DRY RUN: would restore the newest -prev- container.'; return }
        Run 'scp' @($remoteScript, "${SshTarget}:/tmp/drive-remote-deploy.sh")
        Run 'ssh' @($SshTarget, "bash /tmp/drive-remote-deploy.sh rollback '$Container'")
        Write-Host 'Rolled back.' -ForegroundColor Green
        return
    }

    # ---- 1. branch, changes, checks ---------------------------------------
    $cur = (GitOut @('rev-parse', '--abbrev-ref', 'HEAD') | Select-Object -First 1).Trim()
    if ($cur -ne $Branch) { throw "You are on '$cur'. Switch to $Branch (git checkout $Branch) before deploying." }

    $dirty = GitOut @('status', '--porcelain')
    if ($dirty) {
        Step 'Uncommitted changes'
        $dirty | ForEach-Object { Write-Host "   $_" }
        if (-not $Message) { throw "Commit these first, or re-run with -Message 'what changed' to commit and deploy them." }
    }

    if (-not $SkipChecks) {
        Step 'Quick checks (go vet, typecheck)'
        if (Get-Command go -ErrorAction SilentlyContinue) { Run 'go' @('vet', './http/...') } else { Write-Host '   go not found - skipped' -ForegroundColor Yellow }
        if (Get-Command pnpm -ErrorAction SilentlyContinue) {
            Push-Location 'frontend'
            try { Run 'pnpm' @('run', 'typecheck') } finally { Pop-Location }
        } else { Write-Host '   pnpm not found - skipped' -ForegroundColor Yellow }
    }

    if ($DryRun) {
        Write-Host "DRY RUN: would commit (if dirty), push $Branch to GitHub, package HEAD, scp to $SshTarget, build and swap $Container."
        return
    }

    if ($dirty) {
        Step 'Committing'
        Run 'git' @('add', '-A')
        Run 'git' @('commit', '-m', $Message)
    }

    # ---- 2. GitHub must match what we deploy ------------------------------
    Step "Pushing $Branch to GitHub"
    Run 'git' @('fetch', 'origin')
    $behind = [int](GitOut @('rev-list', '--count', "HEAD..origin/$Branch") | Select-Object -First 1)
    if ($behind -gt 0) { throw "GitHub has $behind commit(s) you do not have. Run: git pull --no-rebase origin $Branch  (fix conflicts), then re-run." }
    Run 'git' @('push', 'origin', $Branch)
    $full = (GitOut @('rev-parse', 'HEAD') | Select-Object -First 1).Trim()
    $remoteHead = (GitOut @('rev-parse', "origin/$Branch") | Select-Object -First 1).Trim()
    if ($full -ne $remoteHead) { throw "GitHub ($remoteHead) does not match local HEAD ($full) after push." }
    $sha = $full.Substring(0, 7)
    Write-Host "   GitHub is at $sha  https://github.com/MykMILLI/drive/commit/$full"

    # ---- 3. package and copy ------------------------------------------------
    Step "Packaging $sha"
    $tgz = Join-Path $work "drive-src-$sha.tar.gz"
    Run 'git' @('archive', '--format=tar.gz', '-o', $tgz, 'HEAD')
    Write-Host ("   {0:N1} MB" -f ((Get-Item $tgz).Length / 1MB))

    Step "Copying to $SshTarget"
    Run 'scp' @($tgz, $remoteScript, "${SshTarget}:/tmp/")

    # ---- 4. build + swap on the host ---------------------------------------
    Step 'Building and swapping on the host (the build takes a few minutes)'
    Run 'ssh' @($SshTarget, "bash /tmp/drive-remote-deploy.sh deploy '$Container' '$sha' '/tmp/drive-src-$sha.tar.gz'")

    Write-Host ''
    Write-Host "Deployed $sha. Hard-refresh the browser (Ctrl+F5)." -ForegroundColor Green
    Write-Host 'If anything looks wrong:  .\deploy-drive.ps1 -Rollback' -ForegroundColor Green
}
finally { Pop-Location }
