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
        <h1>{{ t("drive.starred") }}</h1>
        <p>{{ t("drive.starredHint") }}</p>
      </div>
    </div>

    <div v-if="loading" class="empty-state">
      <div class="spinner">
        <div class="bounce1"></div>
        <div class="bounce2"></div>
        <div class="bounce3"></div>
      </div>
    </div>
    <div v-else-if="!items.length" class="empty-state">
      <div class="empty-art"><i class="material-icons">star_outline</i></div>
      <h3>{{ t("drive.starredEmpty") }}</h3>
      <p>{{ t("drive.starredEmptyHint") }}</p>
    </div>
    <flat-list v-else :items="items">
      <template #actions="{ item }">
        <button
          class="action"
          :title="t('drive.unstar')"
          :aria-label="t('drive.unstar')"
          @click="unstar(item.path)"
        >
          <i class="material-icons star-on">star</i>
        </button>
      </template>
    </flat-list>
  </div>
</template>

<script setup lang="ts">
import { inject, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import HeaderBar from "@/components/header/HeaderBar.vue";
import Action from "@/components/header/Action.vue";
import Search from "@/components/Search.vue";
import FlatList from "@/components/files/FlatList.vue";
import { useStarredStore, normalizeStarPath } from "@/stores/starred";

const { t } = useI18n();
const $showError = inject<IToastError>("$showError")!;
const starred = useStarredStore();
const items = ref<RecursiveEntry[]>([]);
const loading = ref(true);

const load = async () => {
  loading.value = true;
  try {
    items.value = (await starred.load(true)) ?? [];
  } catch (e: any) {
    $showError(e);
  } finally {
    loading.value = false;
  }
};

const unstar = async (path: string) => {
  try {
    await starred.toggle(path);
    items.value = items.value.filter(
      (i) => normalizeStarPath(i.path) !== normalizeStarPath(path)
    );
  } catch (e: any) {
    $showError(e);
  }
};

onMounted(load);
</script>
