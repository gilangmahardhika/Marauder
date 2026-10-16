<script lang="ts">
  let { class: className = '' }: { class?: string } = $props();
</script>

<svg class={className} viewBox="0 0 400 220" fill="none" aria-hidden="true" focusable="false">
  <g class="fill-line">
    <circle cx="112" cy="24" r="7" />
    <circle cx="122" cy="19" r="9" />
    <circle cx="132" cy="24" r="7" />
    <rect x="112" y="24" width="20" height="7" />
  </g>
  <g class="stroke-line-strong" stroke-width="2" stroke-linecap="round">
    <path d="M20 16v8M16 20h8" />
    <path d="M384 104v8M380 108h8" />
    <path d="M112 202v8M108 206h8" />
    <path d="M258 10v8M254 14h8" />
  </g>
  <circle cx="14" cy="132" r="3" class="fill-line-strong" />
  <circle cx="392" cy="200" r="4" class="fill-line" />

  <circle cx="200" cy="112" r="78" stroke-width="14" class="stroke-accent/10" />
  <g class="stroke-accent" stroke-width="2.5" stroke-linecap="round">
    <path d="M136.1 67.3A78 78 0 0 1 193.2 34.3" />
    <path d="M267.5 73A78 78 0 0 1 273.3 138.7" />
    <path d="M161 179.5A78 78 0 0 1 123.2 125.5" />
  </g>
  <g class="fill-accent">
    <path d="M200.2 33.7L192.8 29.3L193.6 39.3Z" />
    <path d="M270.9 145.3L278 140.4L268.6 137Z" />
    <path d="M122 118.6L118.3 126.4L128.1 124.6Z" />
  </g>

  <rect x="30" y="40" width="62" height="78" rx="6" stroke-width="1.5" class="fill-surface stroke-line-strong" />
  <text x="38" y="54" font-size="9" font-weight="600" class="fill-accent">PRD</text>
  <g class="fill-line-strong">
    <rect x="38" y="60" width="46" height="3" rx="1.5" />
    <rect x="38" y="68" width="38" height="3" rx="1.5" />
    <rect x="48" y="80" width="34" height="3" rx="1.5" />
    <rect x="48" y="92" width="28" height="3" rx="1.5" />
    <rect x="48" y="104" width="30" height="3" rx="1.5" />
  </g>
  <g class="stroke-accent" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round">
    <path d="M38 81.5l2.5 2.5 4.5-5" />
    <path d="M38 93.5l2.5 2.5 4.5-5" />
  </g>
  <rect x="38.5" y="101.5" width="6" height="6" rx="1" class="stroke-line-strong" />

  <rect x="290" y="22" width="92" height="66" rx="6" class="fill-ink" />
  <circle cx="298" cy="31" r="2.2" class="fill-highlight" />
  <circle cx="305" cy="31" r="2.2" class="fill-line-strong" />
  <circle cx="312" cy="31" r="2.2" class="fill-surface/40" />
  <rect x="298" y="42" width="14" height="3" rx="1.5" class="fill-highlight" />
  <rect x="315" y="42" width="30" height="3" rx="1.5" class="fill-surface/50" />
  <rect x="306" y="50" width="22" height="3" rx="1.5" class="fill-line-strong" />
  <rect x="331" y="50" width="18" height="3" rx="1.5" class="fill-highlight" />
  <rect x="306" y="58" width="36" height="3" rx="1.5" class="fill-surface/50" />
  <rect x="298" y="66" width="10" height="3" rx="1.5" class="fill-highlight" />
  <rect x="311" y="66" width="24" height="3" rx="1.5" class="fill-line-strong" />
  <path
    d="M352 72l-5 4 5 4M366 72l5 4-5 4M361 70l-4 12"
    stroke-width="1.8"
    stroke-linecap="round"
    stroke-linejoin="round"
    class="stroke-highlight"
  />

  <rect x="288" y="138" width="100" height="46" rx="6" stroke-width="1.5" class="fill-surface stroke-line-strong" />
  <path d="M305 155H371" stroke-width="2" class="stroke-line-strong" />
  <circle cx="305" cy="155" r="8" class="fill-accent" />
  <circle cx="338" cy="155" r="8" class="fill-accent" />
  <circle cx="371" cy="155" r="8" class="fill-highlight" />
  <g class="stroke-surface" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
    <path d="M301.5 155l2.5 2.5 4.5-5" />
    <path d="M334.5 155l2.5 2.5 4.5-5" />
    <path d="M371 159v-8M367.5 154.5l3.5-3.5 3.5 3.5" />
  </g>
  <g font-size="7" text-anchor="middle" class="fill-muted">
    <text x="305" y="177">build</text>
    <text x="338" y="177">test</text>
    <text x="371" y="177">deploy</text>
  </g>

  <g stroke-width="1.5" class="fill-surface stroke-line-strong">
    <rect x="34" y="144" width="56" height="18" rx="4" />
    <rect x="34" y="166" width="56" height="18" rx="4" />
  </g>
  <circle cx="44" cy="153" r="2.5" class="fill-accent" />
  <circle cx="44" cy="175" r="2.5" class="fill-highlight" />
  <rect x="52" y="151.5" width="30" height="3" rx="1.5" class="fill-line-strong" />
  <rect x="52" y="173.5" width="30" height="3" rx="1.5" class="fill-line-strong" />

  <circle cx="236" cy="72" r="6" class="fill-highlight" />
  <rect x="233" y="78" width="6" height="4" rx="1" class="fill-muted" />
  <g class="stroke-highlight" stroke-width="1.6" stroke-linecap="round">
    <path d="M236 60v-4M246 66l3-2M226 66l-3-2M248 74h3M224 74h-3" />
  </g>

  <ellipse cx="200" cy="194" rx="64" ry="5" class="fill-line" />
  <rect x="195" y="113" width="10" height="12" class="fill-[#e3b893]" />
  <path d="M170 182V150Q170 128 192 124H208Q230 128 230 150V182Z" class="fill-accent" />
  <path d="M194 124L200 131L206 124Z" class="fill-[#e3b893]" />
  <circle cx="200" cy="100" r="15" class="fill-[#e3b893]" />
  <path d="M185 100A15 15 0 0 1 215 100Q208 92 198 93Q190 94 185 100Z" class="fill-ink" />
  <circle cx="195" cy="103" r="1.3" class="fill-ink" />
  <circle cx="205" cy="103" r="1.3" class="fill-ink" />
  <path d="M196 108q4 3 8 0" stroke-width="1.2" stroke-linecap="round" class="stroke-ink" />
  <rect x="164" y="142" width="72" height="40" rx="4" class="fill-ink" />
  <circle cx="200" cy="162" r="4" class="fill-highlight" />
  <rect x="156" y="181" width="88" height="5" rx="2.5" class="fill-muted" />
</svg>
