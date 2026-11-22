<template>
  <div class="user-menu" ref="root">
    <button
      class="avatar"
      type="button"
      :title="username"
      :aria-label="t('drive.account')"
      :aria-expanded="open"
      aria-haspopup="menu"
      @click.stop="open = !open"
    >
      {{ initial }}
    </button>
    <div v-if="open" class="menu-pop" role="menu">
      <div class="user-card">
        <span class="avatar">{{ initial }}</span>
        <div>
          <strong>{{ username }}</strong>
          <small>{{
            authStore.user?.perm.admin
              ? t("drive.administrator")
              : t("drive.member")
          }}</small>
        </div>
      </div>
      <div class="menu-label">{{ t("drive.appearance") }}</div>
      <div class="theme-switch" role="group">
        <button
          v-for="opt in themes"
          :key="opt.value"
          :class="{ active: current === opt.value }"
          @click="pickTheme(opt.value)"
        >
          <i class="material-icons">{{ opt.icon }}</i
          >{{ opt.label }}
        </button>
      </div>
      <hr />
      <router-link to="/settings/profile" role="menuitem" @click="open = false">
        <i class="material-icons">manage_accounts</i>
        {{ t("settings.profileSettings") }}
      </router-link>
      <router-link
        v-if="authStore.user?.perm.share"
        to="/shared"
        role="menuitem"
        @click="open = false"
      >
        <i class="material-icons">link</i>
        {{ t("drive.sharedLinks") }}
      </router-link>
      <template v-if="canLogout">
        <hr />
        <button role="menuitem" @click="logout">
          <i class="material-icons">logout</i>
          {{ t("sidebar.logout") }}
        </button>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { useAuthStore } from "@/stores/auth";
import { getStoredTheme, setStoredTheme } from "@/utils/theme";
import { noAuth, loginPage, logoutPage } from "@/utils/constants";
import * as auth from "@/utils/auth";

const { t } = useI18n();
const authStore = useAuthStore();
const open = ref(false);
const root = ref<HTMLElement | null>(null);
const current = ref<UserTheme>(getStoredTheme());

const username = computed(() => authStore.user?.username ?? "");
const initial = computed(() => (username.value[0] ?? "?").toUpperCase());
const canLogout = !noAuth && (loginPage || logoutPage !== "/login");

const themes = computed(() => [
  { value: "light" as UserTheme, icon: "light_mode", label: t("drive.light") },
  { value: "dark" as UserTheme, icon: "dark_mode", label: t("drive.dark") },
  { value: "" as UserTheme, icon: "contrast", label: t("drive.auto") },
]);

const pickTheme = (v: UserTheme) => {
  current.value = v;
  setStoredTheme(v);
};

const logout = () => {
  open.value = false;
  auth.logout();
};

const onDoc = (e: MouseEvent) => {
  if (open.value && root.value && !root.value.contains(e.target as Node)) {
    open.value = false;
  }
};

onMounted(() => document.addEventListener("click", onDoc));
onBeforeUnmount(() => document.removeEventListener("click", onDoc));
</script>
