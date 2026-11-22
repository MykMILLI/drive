<template>
  <div class="card floating share-dialog" id="share">
    <div class="card-title share-head">
      <div class="share-file-icon">
        <i class="material-icons">{{ isDir ? "folder" : "description" }}</i>
      </div>
      <div class="share-head-text">
        <h2>{{ $t("drive.shareTitle", { name: itemName }) }}</h2>
        <p>{{ $t("drive.shareSubtitle") }}</p>
      </div>
      <button
        class="action"
        :aria-label="$t('buttons.close')"
        :title="$t('buttons.close')"
        @click="closeHovers"
      >
        <i class="material-icons">close</i>
      </button>
    </div>

    <div class="card-content">
      <!-- Freshly created link -->
      <div v-if="created" class="share-created">
        <div class="share-created-row">
          <i class="material-icons ok">check_circle</i>
          <div class="share-created-text">
            <strong>{{ $t("drive.linkReady") }}</strong>
            <a :href="buildLink(created)" target="_blank" class="link-text">{{
              buildLink(created)
            }}</a>
          </div>
        </div>
        <div class="share-created-actions">
          <button class="button" @click="copyToClipboard(buildLink(created))">
            <i class="material-icons">content_copy</i>
            {{ $t("drive.copyLink") }}
          </button>
          <button
            class="button button--ghost"
            @click="qrFor = qrFor === created.hash ? '' : created.hash"
          >
            <i class="material-icons">qr_code_2</i>
            {{ $t("drive.qrCode") }}
          </button>
          <button class="button button--ghost" @click="created = null">
            <i class="material-icons">add_link</i>
            {{ $t("drive.anotherLink") }}
          </button>
        </div>
        <div v-if="qrFor === created.hash" class="share-qr">
          <qrcode-vue
            :value="buildLink(created)"
            :size="168"
            level="M"
            render-as="svg"
          />
        </div>
      </div>

      <!-- Create form -->
      <form v-else class="share-form" @submit.prevent="submit">
        <div class="share-access">
          <div class="share-access-icon">
            <i class="material-icons">public</i>
          </div>
          <div>
            <strong>{{ $t("drive.anyoneWithLink") }}</strong>
            <p>{{ $t("drive.anyoneWithLinkHint") }}</p>
          </div>
        </div>

        <label class="share-label" for="share-slug">{{
          $t("drive.customLink")
        }}</label>
        <div
          class="slug-field"
          :class="{
            invalid: slugState === 'invalid' || slugState === 'taken',
            valid: slugState === 'available',
          }"
        >
          <span class="slug-prefix" :title="prefix">{{ prefixShort }}</span>
          <input
            id="share-slug"
            v-model="slug"
            @input="onSlugInput"
            :placeholder="$t('drive.slugPlaceholder')"
            autocomplete="off"
            autocapitalize="off"
            spellcheck="false"
            maxlength="64"
          />
          <i v-if="slugState === 'checking'" class="material-icons spin"
            >progress_activity</i
          >
          <i v-else-if="slugState === 'available'" class="material-icons ok"
            >check_circle</i
          >
          <i
            v-else-if="slugState === 'taken' || slugState === 'invalid'"
            class="material-icons bad"
            >error</i
          >
        </div>
        <p
          class="share-hint"
          :class="{
            bad: slugState === 'invalid' || slugState === 'taken',
            ok: slugState === 'available',
          }"
        >
          <template v-if="slugState === 'invalid'">{{
            $t("drive.slugInvalid")
          }}</template>
          <template v-else-if="slugState === 'taken'">{{
            $t("drive.slugTaken")
          }}</template>
          <template v-else-if="slugState === 'available'">{{
            $t("drive.slugAvailable")
          }}</template>
          <template v-else>{{ $t("drive.slugHint") }}</template>
        </p>

        <label class="share-label">{{ $t("drive.expiration") }}</label>
        <div class="chip-row">
          <button
            v-for="p in presets"
            :key="p.key"
            type="button"
            class="chip"
            :class="{ active: preset === p.key }"
            @click="preset = p.key"
          >
            {{ p.label }}
          </button>
        </div>
        <div v-if="preset === 'custom'" class="custom-expiry">
          <input
            class="input"
            type="number"
            min="1"
            max="2147483647"
            v-model.number="time"
            :aria-label="$t('settings.shareDuration')"
          />
          <select class="input" v-model="unit" :aria-label="$t('time.unit')">
            <option value="minutes">{{ $t("time.minutes") }}</option>
            <option value="hours">{{ $t("time.hours") }}</option>
            <option value="days">{{ $t("time.days") }}</option>
          </select>
        </div>

        <label class="switch-row">
          <span>
            <strong>{{ $t("drive.requirePassword") }}</strong>
            <small>{{ $t("drive.requirePasswordHint") }}</small>
          </span>
          <input type="checkbox" class="switch" v-model="usePassword" />
        </label>
        <input
          v-if="usePassword"
          class="input input--block"
          type="password"
          v-model.trim="password"
          :placeholder="$t('drive.password')"
          autocomplete="new-password"
        />
      </form>

      <!-- Existing links -->
      <div v-if="links.length" class="share-existing">
        <div class="share-label">
          {{ $t("drive.existingLinks", { n: links.length }, links.length) }}
        </div>
        <ul>
          <li v-for="link in links" :key="link.hash">
            <div class="share-link-main">
              <i class="material-icons">link</i>
              <div>
                <a :href="buildLink(link)" target="_blank" class="link-text"
                  >/share/{{ link.hash }}</a
                >
                <div class="share-link-meta">
                  <span class="pill" :class="link.expire === 0 ? 'ok' : 'warn'">
                    <i class="material-icons">{{
                      link.expire === 0 ? "all_inclusive" : "schedule"
                    }}</i>
                    {{
                      link.expire === 0
                        ? $t("permanent")
                        : $t("drive.expires", { when: humanTime(link.expire) })
                    }}
                  </span>
                  <span v-if="link.hasPassword" class="pill">
                    <i class="material-icons">lock</i
                    >{{ $t("drive.protected") }}
                  </span>
                </div>
              </div>
            </div>
            <div class="share-link-actions">
              <button
                class="action"
                :title="$t('buttons.copyToClipboard')"
                :aria-label="$t('buttons.copyToClipboard')"
                @click="copyToClipboard(buildLink(link))"
              >
                <i class="material-icons">content_copy</i>
              </button>
              <button
                class="action"
                :title="$t('buttons.copyDownloadLinkToClipboard')"
                :aria-label="$t('buttons.copyDownloadLinkToClipboard')"
                :disabled="!!link.hasPassword"
                @click="copyToClipboard(buildDownloadLink(link))"
              >
                <i class="material-icons">download</i>
              </button>
              <button
                class="action"
                :title="$t('drive.qrCode')"
                :aria-label="$t('drive.qrCode')"
                @click="qrFor = qrFor === link.hash ? '' : link.hash"
              >
                <i class="material-icons">qr_code_2</i>
              </button>
              <button
                class="action danger"
                :title="$t('buttons.delete')"
                :aria-label="$t('buttons.delete')"
                @click="deleteLink($event, link)"
              >
                <i class="material-icons">delete_outline</i>
              </button>
            </div>
            <div
              v-if="
                qrFor === link.hash && (!created || created.hash !== link.hash)
              "
              class="share-qr"
            >
              <qrcode-vue
                :value="buildLink(link)"
                :size="148"
                level="M"
                render-as="svg"
              />
            </div>
          </li>
        </ul>
      </div>
    </div>

    <div class="card-action">
      <button class="button button--flat button--grey" @click="closeHovers">
        {{ $t("buttons.close") }}
      </button>
      <button
        v-if="!created"
        id="focus-prompt"
        class="button"
        :disabled="!canSubmit || submitting"
        @click="submit"
      >
        <i class="material-icons">add_link</i>
        {{ $t("drive.createLink") }}
      </button>
    </div>
  </div>
</template>

<script>
import { mapActions, mapState } from "pinia";
import { useFileStore } from "@/stores/file";
import * as api from "@/api/index";
import dayjs from "dayjs";
import { useLayoutStore } from "@/stores/layout";
import { copy } from "@/utils/clipboard";
import { baseURL } from "@/utils/constants";
import QrcodeVue from "qrcode.vue";

const SLUG_RE = /^[a-z0-9][a-z0-9_-]{2,63}$/;

export default {
  name: "share",
  components: { QrcodeVue },
  data: function () {
    return {
      slug: "",
      slugState: "idle", // idle | checking | available | taken | invalid
      slugTimer: null,
      slugSeq: 0,
      preset: "never",
      time: 7,
      unit: "days",
      usePassword: false,
      password: "",
      links: [],
      created: null,
      qrFor: "",
      submitting: false,
    };
  },
  inject: ["$showError", "$showSuccess"],
  computed: {
    ...mapState(useFileStore, [
      "req",
      "selected",
      "selectedCount",
      "isListing",
    ]),
    item() {
      if (!this.isListing) return this.req;
      if (this.selectedCount !== 1) return null;
      return this.req.items[this.selected[0]];
    },
    url() {
      if (!this.isListing) {
        return this.$route.path;
      }
      if (this.selectedCount === 0 || this.selectedCount > 1) {
        // This shouldn't happen.
        return;
      }
      return this.req.items[this.selected[0]].url;
    },
    itemName() {
      return this.item?.name || "";
    },
    isDir() {
      return !!this.item?.isDir;
    },
    prefix() {
      return `${window.location.origin}${baseURL}/share/`;
    },
    prefixShort() {
      return `${window.location.host}${baseURL}/share/`;
    },
    presets() {
      return [
        { key: "never", label: this.$t("drive.never") },
        { key: "1h", label: this.$t("drive.oneHour") },
        { key: "1d", label: this.$t("drive.oneDay") },
        { key: "7d", label: this.$t("drive.sevenDays") },
        { key: "30d", label: this.$t("drive.thirtyDays") },
        { key: "custom", label: this.$t("drive.custom") },
      ];
    },
    canSubmit() {
      if (this.slug && this.slugState !== "available") return false;
      if (this.usePassword && !this.password) return false;
      if (this.preset === "custom" && !(this.time > 0)) return false;
      return true;
    },
  },
  async beforeMount() {
    try {
      this.links = await api.share.get(this.url);
      this.sort();
    } catch (e) {
      this.$showError(e);
    }
  },
  beforeUnmount() {
    clearTimeout(this.slugTimer);
  },
  methods: {
    ...mapActions(useLayoutStore, ["closeHovers"]),
    onSlugInput() {
      const cleaned = this.slug.toLowerCase().replace(/\s+/g, "-");
      if (cleaned !== this.slug) this.slug = cleaned;
      clearTimeout(this.slugTimer);
      const seq = ++this.slugSeq;
      if (!this.slug) {
        this.slugState = "idle";
        return;
      }
      if (!SLUG_RE.test(this.slug)) {
        this.slugState = "invalid";
        return;
      }
      this.slugState = "checking";
      this.slugTimer = setTimeout(async () => {
        try {
          const res = await api.drive.checkSlug(this.slug);
          if (seq !== this.slugSeq) return;
          this.slugState = res.error
            ? "invalid"
            : res.available
              ? "available"
              : "taken";
        } catch {
          if (seq === this.slugSeq) this.slugState = "idle";
        }
      }, 300);
    },
    expiry() {
      switch (this.preset) {
        case "1h":
          return ["1", "hours"];
        case "1d":
          return ["1", "days"];
        case "7d":
          return ["7", "days"];
        case "30d":
          return ["30", "days"];
        case "custom":
          return [String(this.time), this.unit];
        default:
          return ["", "hours"];
      }
    },
    copyToClipboard: function (text) {
      copy({ text }).then(
        () => this.$showSuccess(this.$t("success.linkCopied")),
        () => {
          copy({ text }, { permission: true }).then(
            () => this.$showSuccess(this.$t("success.linkCopied")),
            (e) => this.$showError(e)
          );
        }
      );
    },
    submit: async function () {
      if (!this.canSubmit || this.submitting) return;
      this.submitting = true;
      try {
        const [expires, unit] = this.expiry();
        const res = await api.share.create(
          this.url,
          this.usePassword ? this.password : "",
          expires,
          unit,
          this.slug
        );
        this.links.push(res);
        this.sort();
        this.created = res;
        this.slug = "";
        this.slugState = "idle";
        this.password = "";
        this.usePassword = false;
        this.preset = "never";
        // Best effort: copy silently, the "Copy link" button is right there.
        copy({ text: this.buildLink(res) }).catch(() => {});
      } catch (e) {
        if (e?.status === 409) {
          this.slugState = "taken";
        } else if (e?.status === 400 && this.slug) {
          this.slugState = "invalid";
        } else {
          this.$showError(e);
        }
      } finally {
        this.submitting = false;
      }
    },
    deleteLink: async function (event, link) {
      event.preventDefault();
      try {
        await api.share.remove(link.hash);
        this.links = this.links.filter((item) => item.hash !== link.hash);
        if (this.created?.hash === link.hash) this.created = null;
      } catch (e) {
        this.$showError(e);
      }
    },
    humanTime(time) {
      return dayjs(time * 1000).fromNow();
    },
    buildLink(share) {
      return api.share.getShareURL(share);
    },
    buildDownloadLink(share) {
      return api.pub.getDownloadURL({ hash: share.hash, path: "" }, true);
    },
    sort() {
      this.links = this.links.sort((a, b) => {
        if (a.expire === 0) return -1;
        if (b.expire === 0) return 1;
        return a.expire - b.expire;
      });
    },
  },
};
</script>

<style scoped>
.share-dialog {
  max-width: 560px;
}
.share-head {
  display: flex;
  align-items: center;
  gap: 14px;
}
.share-head .action {
  margin-left: auto;
}
.share-file-icon {
  width: 42px;
  height: 42px;
  border-radius: 12px;
  background: var(--accent-soft);
  display: flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 auto;
}
.share-file-icon i {
  color: var(--accent);
}
.share-head-text {
  min-width: 0;
}
.share-head-text h2 {
  margin: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.share-head-text p {
  margin: 2px 0 0;
  font-size: 0.88em;
  color: var(--textPrimary);
}
.share-access {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 14px;
  border-radius: 12px;
  background: var(--surfaceSecondary);
  margin: 6px 0 16px;
}
.share-access-icon {
  width: 34px;
  height: 34px;
  border-radius: 50%;
  background: var(--input-green);
  display: flex;
  align-items: center;
  justify-content: center;
}
.share-access-icon i {
  color: var(--green);
  font-size: 20px;
}
.share-access strong {
  display: block;
  font-weight: 600;
}
.share-access p {
  margin: 1px 0 0;
  font-size: 0.85em;
  color: var(--textPrimary);
}
.share-label {
  display: block;
  font-weight: 600;
  font-size: 0.88em;
  margin: 14px 0 6px;
  color: var(--textSecondary);
}
.slug-field {
  display: flex;
  align-items: center;
  border: 1px solid var(--borderPrimary);
  border-radius: 10px;
  background: var(--surfacePrimary);
  overflow: hidden;
  transition:
    border-color 0.12s,
    box-shadow 0.12s;
}
.slug-field:focus-within {
  border-color: var(--accent);
  box-shadow: 0 0 0 3px var(--accent-soft);
}
.slug-field.invalid {
  border-color: var(--red);
  box-shadow: 0 0 0 3px var(--input-red);
}
.slug-field.valid {
  border-color: var(--green);
}
.slug-prefix {
  padding: 0.65em 0.2em 0.65em 0.85em;
  background: var(--surfaceSecondary);
  color: var(--textPrimary);
  font-size: 0.9em;
  white-space: nowrap;
  max-width: 55%;
  overflow: hidden;
  text-overflow: ellipsis;
  border-right: 1px solid var(--borderPrimary);
  padding-right: 0.6em;
}
.slug-field input {
  flex: 1;
  min-width: 0;
  border: 0;
  outline: 0;
  background: transparent;
  padding: 0.65em 0.7em;
  font-size: 0.95em;
  color: var(--textSecondary);
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
}
.slug-field > i {
  font-size: 20px;
  margin-right: 10px;
}
.ok {
  color: var(--green);
}
.bad {
  color: var(--red);
}
.spin {
  color: var(--textMuted);
  animation: spin 1s linear infinite;
}
.share-hint {
  margin: 6px 2px 0;
  font-size: 0.82em;
  color: var(--textPrimary);
}
.share-hint.ok {
  color: var(--green);
}
.share-hint.bad {
  color: var(--red);
}
.chip-row {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.custom-expiry {
  display: flex;
  gap: 8px;
  margin-top: 8px;
}
.custom-expiry input {
  width: 110px;
}
.custom-expiry select {
  flex: 1;
}
.switch-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin: 18px 0 10px;
  cursor: pointer;
}
.switch-row strong {
  display: block;
  font-weight: 600;
  font-size: 0.92em;
}
.switch-row small {
  color: var(--textPrimary);
  font-size: 0.82em;
}
.switch {
  appearance: none;
  width: 40px;
  height: 22px;
  border-radius: 99px;
  background: var(--borderSecondary);
  position: relative;
  cursor: pointer;
  transition: background 0.15s;
  flex: 0 0 auto;
  margin: 0;
}
.switch::after {
  content: "";
  position: absolute;
  top: 3px;
  left: 3px;
  width: 16px;
  height: 16px;
  border-radius: 50%;
  background: #fff;
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.25);
  transition: transform 0.15s;
}
.switch:checked {
  background: var(--accent);
}
.switch:checked::after {
  transform: translateX(18px);
}
.share-created {
  border: 1px solid var(--borderPrimary);
  background: var(--surfaceSecondary);
  border-radius: 14px;
  padding: 14px;
  margin: 6px 0 6px;
}
.share-created-row {
  display: flex;
  gap: 12px;
  align-items: flex-start;
}
.share-created-text {
  min-width: 0;
}
.share-created-text strong {
  display: block;
  margin-bottom: 2px;
}
.share-created-text a {
  word-break: break-all;
}
.share-created-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 12px;
}
.share-created-actions .button,
.card-action .button {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.share-created-actions .button i,
.card-action .button i {
  font-size: 18px;
}
.button:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.share-qr {
  display: flex;
  justify-content: center;
  padding: 14px;
  margin-top: 10px;
  background: #fff;
  border-radius: 12px;
}
.share-existing {
  margin-top: 18px;
  border-top: 1px solid var(--divider);
  padding-top: 4px;
}
.share-existing ul {
  list-style: none;
  margin: 0;
  padding: 0;
  max-height: 240px;
  overflow: auto;
}
.share-existing li {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding: 8px 2px;
  border-bottom: 1px solid var(--divider);
}
.share-existing li:last-child {
  border-bottom: 0;
}
.share-existing li .share-qr {
  flex-basis: 100%;
}
.share-link-main {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}
.share-link-main > i {
  color: var(--textPrimary);
}
.share-link-meta {
  display: flex;
  gap: 6px;
  margin-top: 4px;
  flex-wrap: wrap;
}
.share-link-actions {
  display: flex;
}
.share-link-actions .action i {
  font-size: 19px;
  padding: 7px;
}
.share-link-actions .action.danger:hover i {
  color: var(--red);
}
.share-link-actions .action:disabled {
  opacity: 0.35;
}
</style>
