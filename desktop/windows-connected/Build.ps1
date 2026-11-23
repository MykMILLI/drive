$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot
$csc = Join-Path $env:WINDIR 'Microsoft.NET\Framework64\v4.0.30319\csc.exe'
if (-not (Test-Path $csc)) { $csc = Join-Path $env:WINDIR 'Microsoft.NET\Framework\v4.0.30319\csc.exe' }
if (-not (Test-Path $csc)) { throw 'Windows .NET Framework C# compiler not found.' }
if (Get-Process DriveDesktop -ErrorAction SilentlyContinue) { throw 'Exit the current DriveDesktop from its tray before rebuilding.' }
$arguments = @('/nologo','/target:winexe','/optimize+','/out:DriveDesktop.exe','/win32icon:Drive.ico')
foreach ($reference in @('System.dll','System.Core.dll','System.Drawing.dll','System.Windows.Forms.dll','System.Net.Http.dll','System.Web.Extensions.dll','System.Security.dll','Microsoft.CSharp.dll')) { $arguments += '/reference:' + $reference }
$arguments += 'Program.cs'
& $csc @arguments
if ($LASTEXITCODE -ne 0) { throw 'Compilation failed.' }
Write-Host 'Built DriveDesktop.exe: new-file upload preview. Publish only after server tests pass.'
