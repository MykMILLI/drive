<template>
  <div v-show="active" @click="closeHovers" class="overlay"></div>
  <nav :class="{ active }" :aria-label="$t('drive.navigation')">
    <router-link class="sb-brand" :to="isLoggedIn ? '/files/' : '/login'">
      <img :src="logoURL" alt="" />
      <span>{{ name }}</span>
    </router-link>

    <template v-if="isLoggedIn">
      <div v-if="user.perm.create" class="sb-new" ref="newRef">
        <button
          type="button"
          @click.stop="newOpen = !newOpen"
          :aria-expanded="newOpen"
          aria-haspopup="menu"
        >
          <i class="material-icons">add</i>
          <span>{{ $t("drive.new") }}</span>
        </button>
        <div v-if="newOpen" class="menu-pop" role="menu" style="top: 54px">
          <button role="menuitem" @click="newAction('newDir')">
            <i class="material-icons">create_new_folder</i>
            {{ $t("sidebar.newFolder") }}
          </button>
          <button role="menuitem" @click="newAction('newFile')">
            <i class="material-icons">note_add</i>
            {{ $t("sidebar.newFile") }}
          </button>
          <hr />
          <button role="menuitem" @click="newAction('uploadFile')">
            <i class="material-icons">upload_file</i>
            {{ $t("drive.uploadFiles") }}
          </button>
          <button role="menuitem" @click="newAction('uploadFolder')">
            <i class="material-icons">drive_folder_upload</i>
            {{ $t("drive.uploadFolder") }}
          </button>
        </div>
      </div>

      <div class="sb-links">
        <router-link
          v-for="link in links"
          :key="link.to"
          :to="link.to"
          class="sb-link"
          :class="{ active: isActive(link) }"
          @click="closeHovers"
        >
          <i class="material-icons">{{ link.icon }}</i>
          <span>{{ link.label }}</span>
        </router-link>

        <div class="sb-section">{{ $t("drive.manage") }}</div>
        <router-link
          to="/settings/profile"
          class="sb-link"
          :class="{ active: $route.path.startsWith('/settings/profile') }"
          @click="closeHovers"
        >
          <i class="material-icons">tune</i>
          <span>{{ $t("sidebar.settings") }}</span>
        </router-link>
        <router-link
          v-if="user.perm.admin"
          to="/settings/global"
          class="sb-link"
          :class="{
            active:
              $route.path.startsWith('/settings') &&
              !$route.path.startsWith('/settings/profile'),
          }"
          @click="closeHovers"
        >
          <i class="material-icons">admin_panel_settings</i>
          <span>{{ $t("drive.admin") }}</span>
        </router-link>
        <button
          v-if="canLogout"
          @click="logout"
          class="sb-link"
          id="logout"
          type="button"
        >
          <i class="material-icons">logout</i>
          <span>{{ $t("sidebar.logout") }}</span>
        </button>
      </div>
    </template>
    <div v-else class="sb-links">
      <router-link v-if="!hideLoginButton" class="sb-link" to="/login">
        <i class="material-icons">login</i>
        <span>{{ $t("sidebar.login") }}</span>
      </router-link>
      <router-link v-if="signup" class="sb-link" to="/login">
        <i class="material-icons">person_add</i>
        <span>{{ $t("sidebar.signup") }}</span>
      </router-link>
    </div>

    <div class="sb-spacer"></div>

    <div v-if="isLoggedIn && !disableUsedPercentage" class="sb-storage">
      <div class="sb-storage-head">
        <i class="material-icons">cloud</i>
        {{ $t("drive.storage") }}
      </div>
      <div class="sb-meter" :class="{ warn: usage.usedPercentage >= 85 }">
        <div
          :style="{ width: Math.min(usage.usedPercentage, 100) + '%' }"
        ></div>
      </div>
      <p>
        {{ $t("sidebar.diskUsed", { used: usage.used, total: usage.total }) }}
      </p>
    </div>

    <div class="sb-foot">
      <span>
        <span v-if="disableExternal">{{ name }}</span>
        <a
          v-else
          rel="noopener noreferrer"
          target="_blank"
          href="https://github.com/MykMILLI/drive"
          >{{ name }}</a
        >
        {{ version }}
      </span>
      <a @click="help">{{ $t("sidebar.help") }}</a>
    </div>
  </nav>
</template>

<script>
import { reactive } from "vue";
import { mapActions, mapState } from "pinia";
import { useAuthStore } from "@/stores/auth";
import { useFileStore } from "@/stores/file";
import { useLayoutStore } from "@/stores/layout";

import * as auth from "@/utils/auth";
import {
  version,
  signup,
  hideLoginButton,
  disableExternal,
  disableUsedPercentage,
  noAuth,
  logoutPage,
  loginPage,
  logoURL,
  name,
} from "@/utils/constants";
import { files as api } from "@/api";
import prettyBytes from "pretty-bytes";

const USAGE_DEFAULT = { used: "0 B", total: "0 B", usedPercentage: 0 };

export default {
  name: "sidebar",
  setup() {
    const usage = reactive({ ...USAGE_DEFAULT });
    return { usage, usageAbortController: new AbortController() };
  },
  data() {
    return { newOpen: false };
  },
  inject: ["$showError"],
  computed: {
    ...mapState(useAuthStore, ["user", "isLoggedIn"]),
    ...mapState(useFileStore, ["isFiles", "isListing", "reload"]),
    ...mapState(useLayoutStore, ["currentPromptName"]),
    active() {
      return this.currentPromptName === "sidebar";
    },
    links() {
      return [
        {
          to: "/files/",
          icon: "folder_open",
          label: this.$t("drive.myDrive"),
          match: "/files",
        },
        { to: "/recent", icon: "schedule", label: this.$t("drive.recent") },
        {
          to: "/starred",
          icon: "star_outline",
          label: this.$t("drive.starred"),
        },
        {
          to: "/projects",
          icon: "account_tree",
          label: this.$t("projects.nav"),
        },
        ...(this.user?.perm.share
          ? [
              {
                to: "/shared",
                icon: "link",
                label: this.$t("drive.sharedLinks"),
              },
            ]
          : []),
      ];
    },
    signup: () => signup,
    hideLoginButton: () => hideLoginButton,
    version: () => version,
    name: () => name,
    logoURL: () => logoURL,
    disableExternal: () => disableExternal,
    disableUsedPercentage: () => disableUsedPercentage,
    canLogout: () => !noAuth && (loginPage || logoutPage !== "/login"),
  },
  methods: {
    ...mapActions(useLayoutStore, ["closeHovers", "showHover"]),
    isActive(link) {
      return this.$route.path.startsWith(link.match || link.to);
    },
    onDocClick(e) {
      if (
        this.newOpen &&
        this.$refs.newRef &&
        !this.$refs.newRef.contains(e.target)
      ) {
        this.newOpen = false;
      }
    },
    async waitForListing() {
      if (this.isListing) return true;
      await this.$router.push({ path: "/files/" });
      for (let i = 0; i < 60; i++) {
        if (this.isListing) return true;
        await new Promise((r) => setTimeout(r, 50));
      }
      return this.isListing;
    },
    async newAction(kind) {
      this.newOpen = false;
      this.closeHovers();
      if (!(await this.waitForListing())) return;
      await this.$nextTick();
      if (kind === "uploadFile") {
        document.getElementById("upload-input")?.click();
      } else if (kind === "uploadFolder") {
        document.getElementById("upload-folder-input")?.click();
      } else {
        this.showHover(kind);
      }
    },
    abortOngoingFetchUsage() {
      this.usageAbortController.abort();
    },
    async fetchUsage() {
      let path = "/files/";
      if (this.$route.path.startsWith("/files")) {
        path = this.$route.path.endsWith("/")
          ? this.$route.path
          : this.$route.path + "/";
      }
      let usageStats = USAGE_DEFAULT;
      if (this.disableUsedPercentage || !this.isLoggedIn) {
        return Object.assign(this.usage, usageStats);
      }
      try {
        this.abortOngoingFetchUsage();
        this.usageAbortController = new AbortController();
        const usage = await api.usage(path, this.usageAbortController.signal);
        usageStats = {
          used: prettyBytes(usage.used, { binary: true }),
          total: prettyBytes(usage.total, { binary: true }),
          usedPercentage: usage.total
            ? Math.round((usage.used / usage.total) * 100)
            : 0,
        };
      } catch {
        // keep defaults
      } finally {
        Object.assign(this.usage, usageStats);
      }
    },
    help() {
      this.showHover("help");
    },
    logout: auth.logout,
  },
  watch: {
    $route: {
      handler(to, from) {
        // Usage depends on the folder only for /files; refresh on first load
        // and when browsing.
        if (!from || to.path.startsWith("/files")) {
          this.fetchUsage();
        }
      },
      immediate: true,
    },
  },
  mounted() {
    document.addEventListener("click", this.onDocClick);
  },
  unmounted() {
    document.removeEventListener("click", this.onDocClick);
    this.abortOngoingFetchUsage();
  },
};
</script>
