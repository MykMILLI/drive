<template>
  <div>
    <header-bar showMenu>
      <search />
      <action
        class="search-button"
        icon="search"
        :label="t('buttons.search')"
        show="search"
      />
    </header-bar>

    <div class="page-head">
      <div>
        <h1>{{ t("drive.sharedLinks") }}</h1>
        <p>{{ t("drive.sharedHint") }}</p>
      </div>
    </div>

    <div v-if="loading" class="empty-state">
      <div class="spinner">
        <div class="bounce1"></div>
        <div class="bounce2"></div>
        <div class="bounce3"></div>
      </div>
    </div>
    <div v-else-if="!links.length" class="empty-state">
      <div class="empty-art"><i class="material-icons">link</i></div>
      <h3>{{ t("drive.sharedEmpty") }}</h3>
      <p>{{ t("drive.sharedEmptyHint") }}</p>
    </div>
    <table v-else class="flat-list">
      <thead>
        <tr>
          <th>{{ t("drive.link") }}</th>
          <th class="hide-sm">{{ t("drive.item") }}</th>
          <th class="hide-sm">{{ t("drive.access") }}</th>
          <th v-if="isAdmin" class="hide-sm">{{ t("settings.username") }}</th>
          <th></th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="link in links" :key="link.hash" @click="openItem(link)">
          <td class="fname">
            <div class="fname-inner">
              <span class="ficon"
                ><i class="material-icons" style="color: var(--accent)"
                  >link</i
                ></span
              >
              <a
                class="link-text"
                :href="buildLink(link)"
                target="_blank"
                @click.stop
                >/share/{{ link.hash }}</a
              >
            </div>
          </td>
          <td class="hide-sm">
            <span class="loc" :title="link.path"
              ><i class="material-icons">{{
                link.path.endsWith("/") ? "folder" : "description"
              }}</i
              >{{ link.path }}</span
            >
          </td>
          <td class="hide-sm">
            <span class="pill" :class="link.expire === 0 ? 'ok' : 'warn'">
              {{
                link.expire === 0
                  ? t("permanent")
                  : t("drive.expires", { when: humanTime(link.expire) })
              }}
            </span>
            <span v-if="link.hasPassword" class="pill" style="margin-left: 6px"
              ><i class="material-icons">lock</i
              >{{ t("drive.protected") }}</span
            >
          </td>
          <td v-if="isAdmin" class="hide-sm">{{ link.username }}</td>
          <td class="actions" @click.stop>
            <button
              class="action"
              :title="t('buttons.copyToClipboard')"
              :aria-label="t('buttons.copyToClipboard')"
              @click="copyLink(link)"
            >
              <i class="material-icons">content_copy</i>
            </button>
            <button
              class="action"
              :title="t('buttons.delete')"
              :aria-label="t('buttons.delete')"
              @click="remove(link)"
            >
              <i class="material-icons">delete_outline</i>
            </button>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<script setup lang="ts">
import { computed, inject, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { useRouter } from "vue-router";
import dayjs from "dayjs";
import HeaderBar from "@/components/header/HeaderBar.vue";
import Action from "@/components/header/Action.vue";
import Search from "@/components/Search.vue";
import { share as api, users } from "@/api";
import { useAuthStore } from "@/stores/auth";
import { useLayoutStore } from "@/stores/layout";
import { copy } from "@/utils/clipboard";
import { encodePath } from "@/utils/url";

const { t } = useI18n();
const router = useRouter();
const $showError = inject<IToastError>("$showError")!;
const $showSuccess = inject<IToastSuccess>("$showSuccess")!;
const authStore = useAuthStore();
const layoutStore = useLayoutStore();
const links = ref<Share[]>([]);
const loading = ref(true);
const isAdmin = computed(() => !!authStore.user?.perm.admin);

const buildLink = (s: Share) => api.getShareURL(s);
const humanTime = (time: number) => dayjs(time * 1000).fromNow();

const copyLink = (s: Share) => {
  const text = buildLink(s);
  copy({ text }).then(
    () => $showSuccess(t("success.linkCopied")),
    () =>
      copy({ text }, { permission: true }).then(
        () => $showSuccess(t("success.linkCopied")),
        (e) => $showError(e)
      )
  );
};

const openItem = (s: Share) => {
  if (s.userID !== authStore.user?.id) return;
  router.push("/files" + encodePath(s.path));
};

const remove = (s: Share) => {
  layoutStore.showHover({
    prompt: "share-delete",
    confirm: async () => {
      layoutStore.closeHovers();
      try {
        await api.remove(s.hash);
        links.value = links.value.filter((l) => l.hash !== s.hash);
        $showSuccess(t("settings.shareDeleted"));
      } catch (e: any) {
        $showError(e);
      }
    },
  });
};

onMounted(async () => {
  try {
    const list = await api.list();
    if (isAdmin.value) {
      const map = new Map<number, string>();
      for (const u of await users.getAll()) map.set(u.id, u.username);
      for (const l of list)
        if (l.userID && map.has(l.userID)) l.username = map.get(l.userID);
    }
    links.value = list.sort(
      (a, b) => (b.expire === 0 ? 1 : 0) - (a.expire === 0 ? 1 : 0)
    );
  } catch (e: any) {
    $showError(e);
  } finally {
    loading.value = false;
  }
});
</script>
