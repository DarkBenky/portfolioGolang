<template>
  <div class="chart-wrapper">
    <div ref="chartContainer" class="chart-container"></div>
    
    <!-- Legend / Info display -->
    <div v-if="currentCandle" class="chart-legend">
      <span class="legend-item">O: <span :class="priceClass">{{ formatPrice(currentCandle.open) }}</span></span>
      <span class="legend-item">H: <span :class="priceClass">{{ formatPrice(currentCandle.high) }}</span></span>
      <span class="legend-item">L: <span :class="priceClass">{{ formatPrice(currentCandle.low) }}</span></span>
      <span class="legend-item">C: <span :class="priceClass">{{ formatPrice(currentCandle.close) }}</span></span>
      <span v-if="currentCandle.volume" class="legend-item">V: {{ formatVolume(currentCandle.volume) }}</span>
    </div>
  </div>
</template>

<script>
import { ref, computed, watch, onMounted, onUnmounted, nextTick } from 'vue'
import { createChart, ColorType, CrosshairMode, CandlestickSeries, HistogramSeries, LineSeries, AreaSeries } from 'lightweight-charts'

export default {
  name: 'CandleChart',
  props: {
    // Array of candle data: { timestamp, open, high, low, close, volume? }
    data: {
      type: Array,
      required: true,
      default: () => []
    },
    width: {
      type: Number,
      default: 0 // 0 = auto width
    },
    height: {
      type: Number,
      default: 400
    },
    // Chart theme
    theme: {
      type: String,
      default: 'dark' // 'dark' or 'light'
    },
    // Colors
    bullColor: {
      type: String,
      default: '#26a69a'
    },
    bearColor: {
      type: String,
      default: '#ef5350'
    },
    // Show volume
    showVolume: {
      type: Boolean,
      default: true
    },
    // Price format decimals
    priceDecimals: {
      type: Number,
      default: 2
    },
    // Crosshair mode
    crosshairMode: {
      type: Number,
      default: CrosshairMode.Normal // 0: Normal, 1: Magnet
    }
  },
  emits: ['chart-ready', 'crosshair-move', 'time-range-change'],
  setup(props, { emit, expose }) {
    const chartContainer = ref(null)
    const currentCandle = ref(null)
    
    // Chart instance and series references
    let chart = null
    let candleSeries = null
    let volumeSeries = null
    const additionalSeries = ref([])

    // Computed
    const priceClass = computed(() => {
      if (!currentCandle.value) return ''
      return currentCandle.value.close >= currentCandle.value.open ? 'bull' : 'bear'
    })

    // Theme colors
    const getThemeColors = () => {
      if (props.theme === 'light') {
        return {
          background: '#ffffff',
          textColor: '#333333',
          gridColor: 'rgba(0, 0, 0, 0.1)',
          borderColor: '#cccccc'
        }
      }
      return {
        background: '#1e1e1e',
        textColor: '#d1d4dc',
        gridColor: 'rgba(255, 255, 255, 0.1)',
        borderColor: '#2b2b43'
      }
    }

    // Format functions
    const formatPrice = (price) => {
      if (price === undefined || price === null) return '-'
      return Number(price).toFixed(props.priceDecimals)
    }

    const formatVolume = (volume) => {
      if (volume >= 1e9) return (volume / 1e9).toFixed(2) + 'B'
      if (volume >= 1e6) return (volume / 1e6).toFixed(2) + 'M'
      if (volume >= 1e3) return (volume / 1e3).toFixed(2) + 'K'
      return volume.toString()
    }

    // Process data for lightweight charts format
    const processData = (rawData) => {
      if (!rawData || rawData.length === 0) return { candles: [], volumes: [] }
      
      const candles = []
      const volumes = []
      
      rawData.forEach(d => {
        const time = d.timestamp // Unix timestamp in seconds
        candles.push({
          time,
          open: +d.open,
          high: +d.high,
          low: +d.low,
          close: +d.close
        })
        
        if (d.volume !== undefined) {
          volumes.push({
            time,
            value: +d.volume,
            color: +d.close >= +d.open 
              ? props.bullColor + '80' // 50% opacity
              : props.bearColor + '80'
          })
        }
      })
      
      // Sort by time
      candles.sort((a, b) => a.time - b.time)
      volumes.sort((a, b) => a.time - b.time)
      
      return { candles, volumes }
    }

    // Initialize chart
    const initChart = () => {
      if (!chartContainer.value) return
      
      const colors = getThemeColors()
      
      chart = createChart(chartContainer.value, {
        autoSize: true,
        height: props.height,
        layout: {
          background: { type: ColorType.Solid, color: colors.background },
          textColor: colors.textColor
        },
        grid: {
          vertLines: { color: colors.gridColor },
          horzLines: { color: colors.gridColor }
        },
        crosshair: {
          mode: props.crosshairMode,
          vertLine: {
            width: 1,
            color: 'rgba(224, 227, 235, 0.4)',
            style: 0
          },
          horzLine: {
            width: 1,
            color: 'rgba(224, 227, 235, 0.4)',
            style: 0
          }
        },
        rightPriceScale: {
          borderColor: colors.borderColor,
          scaleMargins: {
            top: 0.05,
            bottom: props.showVolume ? 0.15 : 0.05
          }
        },
        timeScale: {
          borderColor: colors.borderColor,
          timeVisible: true,
          secondsVisible: false
        },
        handleScroll: {
          vertTouchDrag: true,
          horzTouchDrag: true,
          mouseWheel: true,
          pressedMouseMove: true
        },
        handleScale: {
          axisPressedMouseMove: true,
          mouseWheel: true,
          pinch: true
        }
      })

      // Create candlestick series (v5 API)
      candleSeries = chart.addSeries(CandlestickSeries, {
        upColor: props.bullColor,
        downColor: props.bearColor,
        borderUpColor: props.bullColor,
        borderDownColor: props.bearColor,
        wickUpColor: props.bullColor,
        wickDownColor: props.bearColor
      })

      // Create volume series if enabled (v5 API)
      if (props.showVolume) {
        volumeSeries = chart.addSeries(HistogramSeries, {
          color: props.bullColor,
          priceFormat: {
            type: 'volume'
          },
          priceScaleId: 'volume',
          scaleMargins: {
            top: 0.93,
            bottom: 0
          }
        })
      }

      // Subscribe to crosshair move
      chart.subscribeCrosshairMove((param) => {
        if (param.time && param.seriesData) {
          const candleData = param.seriesData.get(candleSeries)
          if (candleData) {
            currentCandle.value = {
              time: param.time,
              open: candleData.open,
              high: candleData.high,
              low: candleData.low,
              close: candleData.close,
              volume: volumeSeries ? param.seriesData.get(volumeSeries)?.value : undefined
            }
            emit('crosshair-move', currentCandle.value)
          }
        } else {
          // Show last candle when not hovering
          if (props.data && props.data.length > 0) {
            const lastData = props.data[props.data.length - 1]
            currentCandle.value = {
              time: lastData.timestamp,
              open: lastData.open,
              high: lastData.high,
              low: lastData.low,
              close: lastData.close,
              volume: lastData.volume
            }
          }
        }
      })

      // Subscribe to visible time range change
      chart.timeScale().subscribeVisibleTimeRangeChange((range) => {
        emit('time-range-change', range)
      })

      // Set initial data
      updateData()

      emit('chart-ready', { chart, candleSeries, volumeSeries })
    }

    // Update chart data
    const updateData = () => {
      if (!candleSeries) return
      
      const { candles, volumes } = processData(props.data)
      
      candleSeries.setData(candles)
      
      if (volumeSeries && volumes.length > 0) {
        volumeSeries.setData(volumes)
      }

      // Show last candle in legend
      if (candles.length > 0) {
        const lastCandle = candles[candles.length - 1]
        const lastVolume = volumes.length > 0 ? volumes[volumes.length - 1] : null
        currentCandle.value = {
          ...lastCandle,
          volume: lastVolume?.value
        }
      }
    }

    // Resize chart
    const resize = (width, height) => {
      if (chart) {
        chart.resize(
          width || chartContainer.value?.clientWidth || props.width,
          height || props.height
        )
      }
    }

    // Add a line series (for moving averages, etc.)
    const addLineSeries = (options = {}) => {
      if (!chart) return null
      
      const series = chart.addSeries(LineSeries, {
        color: options.color || '#2196F3',
        lineWidth: options.lineWidth || 2,
        lineStyle: options.lineStyle || 0,
        priceLineVisible: options.priceLineVisible ?? false,
        lastValueVisible: options.lastValueVisible ?? false,
        ...options
      })
      
      additionalSeries.value.push(series)
      return series
    }

    // Add an area series
    const addAreaSeries = (options = {}) => {
      if (!chart) return null
      
      const series = chart.addSeries(AreaSeries, {
        topColor: options.topColor || 'rgba(33, 150, 243, 0.4)',
        bottomColor: options.bottomColor || 'rgba(33, 150, 243, 0.0)',
        lineColor: options.lineColor || '#2196F3',
        lineWidth: options.lineWidth || 2,
        priceLineVisible: options.priceLineVisible ?? false,
        lastValueVisible: options.lastValueVisible ?? false,
        ...options
      })
      
      additionalSeries.value.push(series)
      return series
    }

    // Add a histogram series
    const addHistogramSeries = (options = {}) => {
      if (!chart) return null
      
      const series = chart.addSeries(HistogramSeries, {
        color: options.color || '#26a69a',
        priceFormat: options.priceFormat || { type: 'volume' },
        priceScaleId: options.priceScaleId || 'volume',
        ...options
      })
      
      additionalSeries.value.push(series)
      return series
    }

    // Add price line to candle series
    const addPriceLine = (options = {}) => {
      if (!candleSeries) return null
      
      return candleSeries.createPriceLine({
        price: options.price || 0,
        color: options.color || '#2196F3',
        lineWidth: options.lineWidth || 1,
        lineStyle: options.lineStyle || 2, // Dashed
        axisLabelVisible: options.axisLabelVisible ?? true,
        title: options.title || '',
        ...options
      })
    }

    // Remove a series
    const removeSeries = (series) => {
      if (chart && series) {
        chart.removeSeries(series)
        const index = additionalSeries.value.indexOf(series)
        if (index > -1) {
          additionalSeries.value.splice(index, 1)
        }
      }
    }

    // Fit content to view
    const fitContent = () => {
      if (chart) {
        chart.timeScale().fitContent()
      }
    }

    // Set visible range
    const setVisibleRange = (from, to) => {
      if (chart) {
        chart.timeScale().setVisibleRange({ from, to })
      }
    }

    // Get chart instance (for advanced usage)
    const getChart = () => chart
    const getCandleSeries = () => candleSeries
    const getVolumeSeries = () => volumeSeries

    // Watch for data changes
    watch(() => props.data, () => {
      updateData()
    }, { deep: true })

    // Watch for theme changes
    watch(() => props.theme, () => {
      if (chart) {
        const colors = getThemeColors()
        chart.applyOptions({
          layout: {
            background: { type: ColorType.Solid, color: colors.background },
            textColor: colors.textColor
          },
          grid: {
            vertLines: { color: colors.gridColor },
            horzLines: { color: colors.gridColor }
          }
        })
      }
    })

    onMounted(() => {
      nextTick(() => {
        initChart()
      })
    })

    onUnmounted(() => {
      if (chart) {
        chart.remove()
        chart = null
      }
    })

    // Expose methods for parent components
    expose({
      addLineSeries,
      addAreaSeries,
      addHistogramSeries,
      addPriceLine,
      removeSeries,
      fitContent,
      setVisibleRange,
      resize,
      getChart,
      getCandleSeries,
      getVolumeSeries
    })

    return {
      chartContainer,
      currentCandle,
      priceClass,
      formatPrice,
      formatVolume
    }
  }
}
</script>

<style scoped>
.chart-wrapper {
  position: relative;
  width: 100%;
}

.chart-container {
  width: 100%;
}

.chart-legend {
  position: absolute;
  top: 10px;
  left: 35px;
  display: flex;
  gap: 12px;
  font-size: 12px;
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
  color: #d1d4dc;
  background: rgba(30, 30, 30, 0.8);
  padding: 6px 10px;
  border-radius: 4px;
  z-index: 10;
}

.legend-item {
  display: inline-flex;
  gap: 4px;
}

.legend-item .bull {
  color: #26a69a;
}

.legend-item .bear {
  color: #ef5350;
}
</style>
