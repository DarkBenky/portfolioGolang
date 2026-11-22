<template>
  <div ref="container" class="line-chart" :style="{ height: height + 'px' }"></div>
</template>

<script>
import { ref, watch, onMounted, onUnmounted } from 'vue'
import { createChart, ColorType, LineSeries, AreaSeries, HistogramSeries } from 'lightweight-charts'

export default {
  name: 'LineChart',
  props: {
    points: { type: Array, default: () => [] },
    kind: { type: String, default: 'line' },
    color: { type: String, default: '#26a69a' },
    negativeColor: { type: String, default: '#ef5350' },
    height: { type: Number, default: 220 },
    suffix: { type: String, default: '' },
    zeroLine: { type: Boolean, default: false },
    decimals: { type: Number, default: 2 },
  },
  setup(props) {
    const container = ref(null)
    let chart = null
    let series = null

    const priceFormat = () => ({
      type: 'custom',
      formatter: (price) => `${Number(price).toFixed(props.decimals)}${props.suffix}`,
    })

    const makeSeries = () => {
      if (!chart) return
      if (series) {
        chart.removeSeries(series)
        series = null
      }
      const options = { priceFormat: priceFormat(), priceLineVisible: false, lastValueVisible: true }
      if (props.kind === 'area') {
        series = chart.addSeries(AreaSeries, {
          ...options,
          lineColor: props.color,
          topColor: `${props.color}55`,
          bottomColor: `${props.color}05`,
          lineWidth: 2,
        })
      } else if (props.kind === 'histogram') {
        series = chart.addSeries(HistogramSeries, { ...options, color: props.color })
      } else {
        series = chart.addSeries(LineSeries, { ...options, color: props.color, lineWidth: 2 })
      }
      applyData()
    }

    const applyData = () => {
      if (!series) return
      if (props.kind === 'histogram') {
        series.setData(props.points.map((point) => ({
          time: point.time,
          value: point.value,
          color: Number(point.value) < 0 ? props.negativeColor : props.color,
        })))
      } else {
        series.setData(props.points.map((point) => ({ time: point.time, value: point.value })))
      }
      chart.timeScale().fitContent()
    }

    onMounted(() => {
      chart = createChart(container.value, {
        autoSize: true,
        layout: {
          background: { type: ColorType.Solid, color: 'transparent' },
          textColor: '#b9bec7',
          attributionLogo: false,
        },
        grid: { vertLines: { color: 'rgba(255,255,255,0.06)' }, horzLines: { color: 'rgba(255,255,255,0.06)' } },
        rightPriceScale: { borderColor: 'rgba(255,255,255,0.1)', scaleMargins: { top: 0.1, bottom: 0.1 } },
        timeScale: { borderColor: 'rgba(255,255,255,0.1)', timeVisible: false, secondsVisible: false },
        handleScroll: { mouseWheel: true, pressedMouseMove: true, horzTouchDrag: true, vertTouchDrag: false },
        handleScale: { axisPressedMouseMove: true, mouseWheel: true, pinch: true },
      })
      makeSeries()
      if (props.zeroLine && series) {
        series.createPriceLine({ price: 0, color: 'rgba(255,255,255,0.25)', lineWidth: 1, lineStyle: 2,
          axisLabelVisible: false, title: '' })
      }
    })

    watch(() => props.points, applyData, { deep: true })
    watch(() => [props.kind, props.color], makeSeries)

    onUnmounted(() => {
      if (chart) chart.remove()
      chart = null
    })

    return { container }
  },
}
</script>

<style scoped>
.line-chart {
  width: 100%;
}
</style>
