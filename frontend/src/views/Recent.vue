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
        <h1>{{ t("drive.recent") }}</h1>
        <p>{{ t("drive.recentHint") }}</p>
      </div>
      <button class="button button--ghost" @click="load" :disabled="loading">
        {{ t("drive.refresh") }}
      </button>
    </div>

    <div v-if="loading" class="empty-state">
      <div class="spinner">
        <div class="bounce1"></div>
        <div class="bounce2"></div>
        <div class="bounce3"></div>
      </div>
    </div>
    <div v-else-if="!items.length" class="empty-state">
      <div class="empty-art"><i class="material-icons">schedule</i></div>
      <h3>{{ t("drive.recentEmpty") }}</h3>
      <p>{{ t("drive.recentEmptyHint") }}</p>
    </div>
    <template v-else>
      <template v-for="group in groups" :key="group.label">
        <h2 class="group-label">{{ group.label }}</h2>
        <flat-list :items="group.items" />
      </template>
      <p v-if="partial" class="share-hint muted-note">
        {{ t("drive.recentPartial") }}
      </p>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, inject, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import dayjs from "dayjs";
import HeaderBar from "@/components/header/HeaderBar.vue";
import Action from "@/components/header/Action.vue";
import Search from "@/components/Search.vue";
import FlatList from "@/components/files/FlatList.vue";
import { drive as api } from "@/api";

const { t } = useI18n();
const $showError = inject<IToastError>("$showError")!;
const items = ref<RecursiveEntry[]>([]);
const partial = ref(false);
const loading = ref(true);

const groups = computed(() => {
  const today = dayjs().startOf("day");
  const week = today.subtract(7, "day");
  const month = today.subtract(30, "day");
  const buckets = [
    { label: t("drive.today"), items: [] as RecursiveEntry[] },
    { label: t("drive.thisWeek"), items: [] as RecursiveEntry[] },
    { label: t("drive.thisMonth"), items: [] as RecursiveEntry[] },
    { label: t("drive.earlier"), items: [] as RecursiveEntry[] },
  ];
  for (const it of items.value) {
    const d = dayjs(it.modified);
    if (d.isAfter(today)) buckets[0].items.push(it);
    else if (d.isAfter(week)) buckets[1].items.push(it);
    else if (d.isAfter(month)) buckets[2].items.push(it);
    else buckets[3].items.push(it);
  }
  return buckets.filter((b) => b.items.length);
});

const load = async () => {
  loading.value = true;
  try {
    const res = await api.recent(100);
    items.value = res.items;
    partial.value = res.partial;
  } catch (e: any) {
    $showError(e);
  } finally {
    loading.value = false;
  }
};

onMounted(load);
</script>
