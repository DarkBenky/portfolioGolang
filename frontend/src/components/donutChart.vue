<template>
  <div class="donut">
    <div class="donut-graphic" :style="{ width: size + 'px', height: size + 'px' }">
      <svg :width="size" :height="size" :viewBox="`0 0 ${size} ${size}`">
        <circle
          :cx="size / 2"
          :cy="size / 2"
          :r="radius"
          fill="none"
          stroke="rgba(255,255,255,0.06)"
          :stroke-width="thickness"
        />
        <circle
          v-for="slice in slices"
          :key="slice.name"
          :cx="size / 2"
          :cy="size / 2"
          :r="radius"
          fill="none"
          :stroke="slice.color"
          :stroke-width="thickness"
          :stroke-dasharray="`${slice.length} ${circumference - slice.length}`"
          :stroke-dashoffset="-slice.offset"
          :transform="`rotate(-90 ${size / 2} ${size / 2})`"
          stroke-linecap="butt"
        >
          <title>{{ slice.name }}: {{ slice.value.toFixed(1) }}%</title>
        </circle>
      </svg>
      <div class="donut-center">
        <div class="donut-label">{{ centerLabel }}</div>
        <div class="donut-value">{{ centerValue }}</div>
      </div>
    </div>
    <div v-if="showLegend" class="donut-legend">
      <div v-for="slice in slices" :key="'l-' + slice.name" class="legend-row">
        <span class="legend-dot" :style="{ background: slice.color }"></span>
        <span class="legend-name">{{ slice.name }}</span>
        <span class="legend-value">{{ slice.value.toFixed(1) }}%</span>
        <span v-if="legendBars" class="legend-track">
          <span class="legend-bar" :style="{ width: slice.value + '%', background: slice.color }"></span>
        </span>
      </div>
    </div>
  </div>
</template>

<script>
const palette = ['#26a69a', '#2196f3', '#ff9800', '#ab47bc', '#ef5350', '#42a5f5',
  '#ffca28', '#66bb6a', '#8d6e63', '#78909c']

export default {
  name: 'DonutChart',
  props: {
    items: { type: Array, default: () => [] },
    size: { type: Number, default: 170 },
    thickness: { type: Number, default: 22 },
    centerLabel: { type: String, default: '' },
    centerValue: { type: String, default: '' },
    showLegend: { type: Boolean, default: true },
    legendBars: { type: Boolean, default: true },
    maxSlices: { type: Number, default: 7 },
  },
  computed: {
    radius() {
      return (this.size - this.thickness) / 2
    },
    circumference() {
      return 2 * Math.PI * this.radius
    },
    slices() {
      const sorted = [...this.items]
        .map((item) => ({ name: item.name, value: Number(item.percentage ?? item.value) || 0 }))
        .filter((item) => item.value > 0)
        .sort((a, b) => b.value - a.value)
      if (!sorted.length) return []
      const visible = sorted.slice(0, this.maxSlices)
      const rest = sorted.slice(this.maxSlices)
      if (rest.length) {
        visible.push({ name: 'Other', value: rest.reduce((sum, item) => sum + item.value, 0) })
      }
      let offset = 0
      return visible.map((item, index) => {
        const length = (item.value / 100) * this.circumference
        const slice = { ...item, color: palette[index % palette.length], length, offset }
        offset += length
        return slice
      })
    },
  },
}
</script>

<style scoped>
.donut {
  display: flex;
  align-items: center;
  gap: 18px;
  flex-wrap: wrap;
}

.donut-graphic {
  position: relative;
  flex-shrink: 0;
}

.donut-center {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  text-align: center;
}

.donut-label {
  font-size: 10px;
  letter-spacing: 0.05em;
  text-transform: uppercase;
  color: rgba(255, 255, 255, 0.55);
}

.donut-value {
  font-size: 16px;
  font-weight: 600;
}

.donut-legend {
  flex: 1;
  min-width: 130px;
}

.legend-row {
  display: grid;
  grid-template-columns: 8px 1fr auto;
  align-items: center;
  gap: 6px 8px;
  font-size: 12px;
  padding: 3px 0;
}

.legend-track {
  grid-column: 1 / -1;
  height: 3px;
  border-radius: 2px;
  background: rgba(255, 255, 255, 0.06);
  overflow: hidden;
}

.legend-bar {
  display: block;
  height: 100%;
  border-radius: 2px;
}

.legend-dot {
  width: 8px;
  height: 8px;
  border-radius: 2px;
  flex-shrink: 0;
}

.legend-name {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: rgba(255, 255, 255, 0.8);
}

.legend-value {
  color: rgba(255, 255, 255, 0.6);
}
</style>
