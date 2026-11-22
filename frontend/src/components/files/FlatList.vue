<template>
  <table class="flat-list">
    <thead>
      <tr>
        <th>{{ t("files.name") }}</th>
        <th v-if="showLocation" class="hide-sm">{{ t("drive.location") }}</th>
        <th class="hide-sm">{{ t("files.lastModified") }}</th>
        <th class="hide-sm">{{ t("files.size") }}</th>
        <th v-if="$slots.actions"></th>
      </tr>
    </thead>
    <tbody>
      <tr
        v-for="item in items"
        :key="item.path"
        tabindex="0"
        @click="open(item)"
        @keydown.enter="open(item)"
      >
        <td class="fname">
          <div class="fname-inner">
            <span
              class="ficon"
              :data-dir="item.isDir"
              :data-type="kind(item)"
              :data-ext="ext(item)"
              ><i class="material-icons">{{ icon(item) }}</i></span
            >
            <span :title="item.name">{{ item.name }}</span>
            <slot name="badge" :item="item" />
          </div>
        </td>
        <td v-if="showLocation" class="hide-sm">
          <span class="loc" :title="parent(item)"
            ><i class="material-icons">folder</i>{{ parentLabel(item) }}</span
          >
        </td>
        <td class="hide-sm">
          <time :datetime="item.modified" :title="exact(item.modified)">{{
            relative(item.modified)
          }}</time>
        </td>
        <td class="hide-sm">{{ item.isDir ? "—" : filesize(item.size) }}</td>
        <td v-if="$slots.actions" class="actions" @click.stop>
          <slot name="actions" :item="item" />
        </td>
      </tr>
    </tbody>
  </table>
</template>

<script setup lang="ts">
import { useI18n } from "vue-i18n";
import { useRouter } from "vue-router";
import dayjs from "dayjs";
import { filesize } from "@/utils";
import { encodePath } from "@/utils/url";

export interface FlatItem {
  path: string;
  name: string;
  size: number;
  modified: string;
  isDir: boolean;
}

withDefaults(defineProps<{ items: FlatItem[]; showLocation?: boolean }>(), {
  showLocation: true,
});

const { t } = useI18n();
const router = useRouter();

const IMAGE = [
  "jpg",
  "jpeg",
  "png",
  "gif",
  "webp",
  "svg",
  "bmp",
  "heic",
  "avif",
];
const VIDEO = ["mp4", "mkv", "mov", "webm", "avi", "m4v"];
const AUDIO = ["mp3", "flac", "wav", "ogg", "m4a", "aac", "opus"];
const ARCHIVE = ["zip", "rar", "7z", "tar", "gz", "bz2", "xz"];
const DOC = ["doc", "docx", "odt", "rtf", "txt", "md"];
const SHEET = ["xls", "xlsx", "csv", "ods"];
const CODE = [
  "js",
  "ts",
  "go",
  "py",
  "json",
  "html",
  "css",
  "sh",
  "yml",
  "yaml",
  "vue",
];

const extOf = (n: string) => {
  const i = n.lastIndexOf(".");
  return i > 0 ? n.slice(i + 1).toLowerCase() : "";
};

const ext = (item: FlatItem) => (item.isDir ? "" : "." + extOf(item.name));

const kind = (item: FlatItem) => {
  if (item.isDir) return "dir";
  const e = extOf(item.name);
  if (IMAGE.includes(e)) return "image";
  if (VIDEO.includes(e)) return "video";
  if (AUDIO.includes(e)) return "audio";
  if (e === "pdf") return "pdf";
  return "blob";
};

const icon = (item: FlatItem) => {
  if (item.isDir) return "folder";
  const e = extOf(item.name);
  if (IMAGE.includes(e)) return "image";
  if (VIDEO.includes(e)) return "movie";
  if (AUDIO.includes(e)) return "music_note";
  if (ARCHIVE.includes(e)) return "folder_zip";
  if (e === "pdf") return "picture_as_pdf";
  if (SHEET.includes(e)) return "table_chart";
  if (CODE.includes(e)) return "code";
  if (DOC.includes(e)) return "article";
  return "draft";
};

const parent = (item: FlatItem) => {
  const p = item.path.replace(/\/+$/, "");
  const i = p.lastIndexOf("/");
  return i <= 0 ? "/" : p.slice(0, i);
};

const parentLabel = (item: FlatItem) => {
  const p = parent(item);
  return p === "/" ? t("drive.myDrive") : p.split("/").pop();
};

const relative = (d: string) => dayjs(d).fromNow();
const exact = (d: string) => dayjs(d).format("LLL");

const open = (item: FlatItem) => {
  let p =
    "/files" +
    encodePath(item.path.startsWith("/") ? item.path : "/" + item.path);
  if (item.isDir && !p.endsWith("/")) p += "/";
  router.push(p);
};
</script>
