<template>
  <div id="login" class="auth" :class="{ recaptcha: recaptcha }">
    <aside class="auth-hero" aria-hidden="true">
      <div class="auth-hero-inner">
        <div class="auth-brand">
          <img :src="logoURL" alt="" />
          <span>{{ name }}</span>
        </div>
        <h2>{{ t("drive.heroTitle") }}</h2>
        <p>{{ t("drive.heroText") }}</p>
        <ul>
          <li>
            <i class="material-icons">link</i>{{ t("drive.heroFeature1") }}
          </li>
          <li>
            <i class="material-icons">star</i>{{ t("drive.heroFeature2") }}
          </li>
          <li>
            <i class="material-icons">lock</i>{{ t("drive.heroFeature3") }}
          </li>
        </ul>
      </div>
    </aside>

    <section class="auth-panel">
      <div class="auth-card">
        <div class="auth-mobile-brand">
          <img :src="logoURL" alt="" />
        </div>
        <h1>
          {{ createMode ? t("drive.createAccount") : t("drive.welcome") }}
        </h1>
        <p class="auth-sub">{{ t("drive.signInTo", { name }) }}</p>

        <p v-if="reason != null" class="auth-note">
          <i class="material-icons">info</i>
          {{ t(`login.logout_reasons.${reason}`) }}
        </p>
        <p v-if="error !== ''" class="auth-error" role="alert">
          <i class="material-icons">error</i>{{ error }}
        </p>
        <p v-if="info !== ''" class="auth-info" role="status">
          <i class="material-icons">mark_email_read</i>{{ info }}
        </p>

        <div v-if="exchanging" class="auth-exchanging">
          <div class="spinner">
            <div class="bounce1"></div>
            <div class="bounce2"></div>
            <div class="bounce3"></div>
          </div>
          <span>{{ t("drive.signingIn") }}</span>
        </div>

        <template v-else>
          <div v-if="providers.length && !createMode" class="auth-providers">
            <button
              v-for="p in providers"
              :key="p"
              type="button"
              class="provider-btn"
              @click="oauth(p)"
            >
              <span class="provider-mark" :data-provider="p">{{
                providerInitial(p)
              }}</span>
              {{ t("drive.continueWith", { provider: providerLabel(p) }) }}
            </button>
            <div class="auth-divider">
              <span>{{ t("drive.orContinue") }}</span>
            </div>
          </div>

          <form v-if="magicMode" @submit.prevent="sendMagic" class="auth-form">
            <label for="login-email">{{ t("drive.email") }}</label>
            <input
              id="login-email"
              autofocus
              class="input input--block"
              type="email"
              autocomplete="email"
              v-model.trim="username"
              placeholder="you@example.com"
              required
            />
            <button class="button button--block" type="submit" :disabled="busy">
              {{ t("drive.magicLink") }}
            </button>
            <p class="auth-switch">
              <a @click="magicMode = false">{{ t("drive.usePassword") }}</a>
            </p>
          </form>

          <form v-else @submit="submit" class="auth-form">
            <label for="login-username">{{
              supabase && !supabase.AllowLocal
                ? t("drive.email")
                : supabase
                  ? t("drive.username")
                  : t("login.username")
            }}</label>
            <input
              id="login-username"
              autofocus
              class="input input--block"
              :type="supabase && !supabase.AllowLocal ? 'email' : 'text'"
              autocapitalize="off"
              autocomplete="username"
              v-model="username"
            />
            <label for="login-password">{{ t("login.password") }}</label>
            <input
              id="login-password"
              class="input input--block"
              type="password"
              autocomplete="current-password"
              v-model="password"
            />
            <template v-if="createMode">
              <label for="login-password2">{{
                t("login.passwordConfirm")
              }}</label>
              <input
                id="login-password2"
                class="input input--block"
                type="password"
                autocomplete="new-password"
                v-model="passwordConfirm"
              />
            </template>

            <div v-if="recaptcha" id="recaptcha"></div>
            <button class="button button--block" type="submit" :disabled="busy">
              {{ createMode ? t("login.signup") : t("login.submit") }}
            </button>

            <p v-if="supabase?.MagicLink && !createMode" class="auth-switch">
              <a @click="magicMode = true">{{ t("drive.useMagicLink") }}</a>
            </p>
          </form>

          <p v-if="signup" class="auth-switch">
            {{ createMode ? t("drive.haveAccount") : t("drive.noAccount") }}
            <a @click="toggleMode">{{
              createMode ? t("login.loginInstead") : t("login.createAnAccount")
            }}</a>
          </p>
        </template>

        <p v-if="supabase" class="auth-foot">
          <i class="material-icons">verified_user</i>{{ t("drive.securedBy") }}
        </p>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { StatusError } from "@/api/utils";
import { drive as driveApi } from "@/api";
import * as auth from "@/utils/auth";
import {
  name,
  logoURL,
  recaptcha,
  recaptchaKey,
  signup,
  supabase,
  baseURL,
} from "@/utils/constants";
import { computed, inject, onBeforeUnmount, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { useRoute, useRouter } from "vue-router";

const REDIRECT_KEY = "drive-login-redirect";

const createMode = ref<boolean>(false);
const magicMode = ref<boolean>(false);
const error = ref<string>("");
const info = ref<string>("");
const username = ref<string>("");
const password = ref<string>("");
const passwordConfirm = ref<string>("");
const busy = ref<boolean>(false);
const exchanging = ref<boolean>(false);

const route = useRoute();
const router = useRouter();
const { t } = useI18n({});
const toggleMode = () => (createMode.value = !createMode.value);

const $showError = inject<IToastError>("$showError")!;

const reason = route.query["logout-reason"] ?? null;

const providers = computed(() => supabase?.Providers ?? []);

const PROVIDER_LABELS: Record<string, string> = {
  google: "Google",
  github: "GitHub",
  gitlab: "GitLab",
  azure: "Microsoft",
  apple: "Apple",
  discord: "Discord",
  facebook: "Facebook",
  twitter: "X",
  slack: "Slack",
  keycloak: "Keycloak",
  bitbucket: "Bitbucket",
  linkedin_oidc: "LinkedIn",
};
const providerLabel = (p: string) =>
  PROVIDER_LABELS[p] ?? p.charAt(0).toUpperCase() + p.slice(1);
const providerInitial = (p: string) => providerLabel(p).charAt(0);

const redirectTarget = () => {
  const r = route.query.redirect as string | undefined;
  if (r && r.startsWith("/")) return r;
  const stored = sessionStorage.getItem(REDIRECT_KEY);
  if (stored && stored.startsWith("/")) return stored;
  return "/files/";
};

const loginURL = () => `${window.location.origin}${baseURL}/login`;

const oauth = (provider: string) => {
  if (!supabase) return;
  sessionStorage.setItem(REDIRECT_KEY, redirectTarget());
  const params = new URLSearchParams({ provider, redirect_to: loginURL() });
  window.location.href = `${supabase.URL}/auth/v1/authorize?${params}`;
};

const sendMagic = async () => {
  error.value = "";
  info.value = "";
  if (!username.value) return;
  busy.value = true;
  try {
    sessionStorage.setItem(REDIRECT_KEY, redirectTarget());
    await driveApi.sendMagicLink(username.value);
    info.value = t("drive.magicLinkSent", { email: username.value });
  } catch (e: any) {
    error.value = e?.message || String(e);
  } finally {
    busy.value = false;
  }
};

const submit = async (event: Event) => {
  event.preventDefault();
  event.stopPropagation();
  error.value = "";

  const redirect = redirectTarget();

  let captcha = "";
  if (recaptcha) {
    captcha = window.grecaptcha.getResponse();

    if (captcha === "") {
      error.value = t("login.wrongCredentials");
      return;
    }
  }

  if (createMode.value) {
    if (password.value !== passwordConfirm.value) {
      error.value = t("login.passwordsDontMatch");
      return;
    }
  }

  busy.value = true;
  try {
    if (createMode.value) {
      await auth.signup(username.value, password.value);
    }

    await auth.login(username.value, password.value, captcha);
    sessionStorage.removeItem(REDIRECT_KEY);
    router.push({ path: redirect });
  } catch (e: any) {
    if (e instanceof StatusError) {
      if (e.status === 409) {
        error.value = t("login.usernameTaken");
      } else if (e.status === 403) {
        error.value = t("login.wrongCredentials");
      } else if (e.status === 400) {
        const match = e.message.match(/minimum length is (\d+)/);
        if (match) {
          error.value = t("login.passwordTooShort", { min: match[1] });
        } else {
          error.value = e.message;
        }
      } else {
        $showError(e);
      }
    }
  } finally {
    busy.value = false;
  }
};

onBeforeUnmount(() =>
  window.removeEventListener("hashchange", handleSupabaseReturn)
);

/** Handles the redirect back from Supabase (OAuth or magic link). */
const handleSupabaseReturn = async () => {
  const hash = window.location.hash.replace(/^#/, "");
  const query = new URLSearchParams(window.location.search);
  if (!hash && !query.get("error_description")) return;
  const params = new URLSearchParams(hash);
  const desc =
    params.get("error_description") || query.get("error_description");
  const token = params.get("access_token");
  if (!desc && !token) return;

  // Strip tokens from the address bar right away.
  window.history.replaceState(null, "", window.location.pathname);

  if (desc) {
    error.value = desc.replace(/\+/g, " ");
    return;
  }

  exchanging.value = true;
  try {
    await auth.loginWithToken(token!);
    const redirect = redirectTarget();
    sessionStorage.removeItem(REDIRECT_KEY);
    router.push({ path: redirect });
  } catch (e: any) {
    exchanging.value = false;
    if (e instanceof StatusError && e.status === 403) {
      error.value = t("login.wrongCredentials");
    } else {
      error.value = e?.message || String(e);
    }
  }
};

onMounted(() => {
  if (supabase) {
    handleSupabaseReturn();
    window.addEventListener("hashchange", handleSupabaseReturn);
  }

  if (!recaptcha) return;

  window.grecaptcha.ready(function () {
    window.grecaptcha.render("recaptcha", {
      sitekey: recaptchaKey,
    });
  });
});
</script>

<style scoped>
.auth {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1.1fr);
  min-height: 100vh;
  background: var(--background);
  position: fixed;
  inset: 0;
  overflow: auto;
}
.auth-hero {
  position: relative;
  overflow: hidden;
  background:
    radial-gradient(
      1200px 600px at -10% -10%,
      rgba(124, 92, 255, 0.55),
      transparent 60%
    ),
    radial-gradient(
      900px 500px at 110% 110%,
      rgba(61, 123, 255, 0.6),
      transparent 60%
    ),
    #0f1630;
  color: #fff;
  display: flex;
  align-items: center;
  padding: 56px;
}
.auth-hero-inner {
  max-width: 440px;
}
.auth-brand {
  display: flex;
  align-items: center;
  gap: 12px;
  font-weight: 650;
  font-size: 1.3em;
  margin-bottom: 48px;
}
.auth-brand img {
  width: 44px;
  height: 44px;
  border-radius: 12px;
}
.auth-hero h2 {
  font-size: 2.3em;
  line-height: 1.15;
  letter-spacing: -0.02em;
  margin: 0 0 14px;
  font-weight: 700;
}
.auth-hero p {
  color: rgba(255, 255, 255, 0.75);
  font-size: 1.06em;
  margin: 0 0 28px;
}
.auth-hero ul {
  list-style: none;
  padding: 0;
  margin: 0;
  display: grid;
  gap: 12px;
}
.auth-hero li {
  display: flex;
  align-items: center;
  gap: 12px;
  color: rgba(255, 255, 255, 0.9);
}
.auth-hero li i {
  width: 34px;
  height: 34px;
  border-radius: 10px;
  background: rgba(255, 255, 255, 0.12);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 19px;
}
.auth-panel {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 32px 20px;
}
.auth-card {
  width: 100%;
  max-width: 400px;
}
.auth-mobile-brand {
  display: none;
}
.auth-card h1 {
  font-size: 1.85em;
  letter-spacing: -0.02em;
  margin: 0 0 6px;
  font-weight: 700;
  text-align: left;
}
.auth-sub {
  margin: 0 0 24px;
  color: var(--textPrimary);
}
.auth-form label {
  display: block;
  font-size: 0.88em;
  font-weight: 600;
  margin: 14px 0 6px;
  color: var(--textSecondary);
}
.auth-form .input {
  margin: 0;
  padding: 0.75em 0.95em;
}
.auth-form .button {
  margin-top: 22px;
  padding: 0.8em 1em;
  font-size: 1em;
}
.auth-form .button:disabled {
  opacity: 0.6;
}
.auth-providers {
  display: grid;
  gap: 10px;
}
.provider-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  padding: 0.75em 1em;
  border-radius: 10px;
  border: 1px solid var(--borderPrimary);
  background: var(--surfacePrimary);
  color: var(--textSecondary);
  font-weight: 550;
  font-size: 0.97em;
  cursor: pointer;
  box-shadow: var(--shadow-sm);
}
.provider-btn:hover {
  background: var(--surfaceSecondary);
}
.provider-mark {
  width: 22px;
  height: 22px;
  border-radius: 6px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 0.78em;
  font-weight: 700;
  color: #fff;
  background: var(--textSecondary);
}
.provider-mark[data-provider="google"] {
  background: conic-gradient(
    #ea4335 0 25%,
    #fbbc05 0 50%,
    #34a853 0 75%,
    #4285f4 0
  );
}
.provider-mark[data-provider="github"] {
  background: #24292f;
}
.provider-mark[data-provider="azure"] {
  background: #0078d4;
}
.provider-mark[data-provider="discord"] {
  background: #5865f2;
}
.auth-divider {
  display: flex;
  align-items: center;
  gap: 12px;
  color: var(--textMuted);
  font-size: 0.82em;
  margin: 10px 0 2px;
}
.auth-divider::before,
.auth-divider::after {
  content: "";
  flex: 1;
  border-top: 1px solid var(--borderPrimary);
}
.auth-switch {
  text-align: center;
  margin: 18px 0 0;
  color: var(--textPrimary);
  font-size: 0.92em;
  text-transform: none;
  cursor: default;
  font-weight: 400;
}
.auth-switch a {
  color: var(--accent);
  font-weight: 600;
  cursor: pointer;
}
.auth-error,
.auth-info,
.auth-note {
  display: flex;
  gap: 8px;
  align-items: flex-start;
  padding: 10px 12px;
  border-radius: 10px;
  font-size: 0.92em;
  margin: 0 0 14px;
  text-align: left;
  cursor: default;
  text-transform: none;
  font-weight: 500;
}
.auth-error i,
.auth-info i,
.auth-note i {
  font-size: 19px;
}
.auth-error {
  background: var(--input-red);
  color: var(--red);
}
.auth-info {
  background: var(--input-green);
  color: var(--green);
}
.auth-note {
  background: rgba(245, 165, 36, 0.14);
  color: #b7791f;
}
.auth-exchanging {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 14px;
  padding: 30px 0;
  color: var(--textPrimary);
}
.auth-foot {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  margin: 28px 0 0;
  font-size: 0.8em;
  color: var(--textMuted);
  text-transform: none;
  cursor: default;
  font-weight: 400;
}
.auth-foot i {
  font-size: 16px;
}
#recaptcha {
  margin-top: 14px;
}
@media (max-width: 860px) {
  .auth {
    grid-template-columns: 1fr;
  }
  .auth-hero {
    display: none;
  }
  .auth-mobile-brand {
    display: block;
    margin-bottom: 22px;
  }
  .auth-mobile-brand img {
    width: 48px;
    height: 48px;
    border-radius: 13px;
    margin: 0;
  }
}
</style>
