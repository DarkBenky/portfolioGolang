<template>
  <div class="chart-wrapper">
    <div ref="chartContainer" class="chart-container"></div>
    
    <div v-if="currentData" class="chart-legend">
      <span class="legend-item">Date: <span class="font-weight-bold">{{ currentData.time }}</span></span>
      <span class="legend-item">Portfolio: <span class="text-primary font-weight-bold">{{ currentData.portfolio }}%</span></span>
      <span class="legend-item">{{ benchmarkName }}: <span class="text-warning font-weight-bold">{{ currentData.benchmark }}%</span></span>
      <span class="legend-item">
        Diff: <span :class="currentData.diff >= 0 ? 'text-success' : 'text-error'" class="font-weight-bold">
          {{ currentData.diff >= 0 ? '+' : '' }}{{ currentData.diff }}%
        </span>
      </span>
    </div>
  </div>
</template>

<script>
import { ref, watch, onMounted, onUnmounted, nextTick } from 'vue'
import { createChart, ColorType, LineSeries } from 'lightweight-charts'

export default {
  name: 'BacktestChart',
  props: {
    portfolioData: {
      type: Array,
      required: true,
      default: () => []
    },
    benchmarkData: {
      type: Array,
      required: true,
      default: () => []
    },
    timestamps: {
      type: Array,
      required: true,
      default: () => []
    },
    benchmarkName: {
      type: String,
      default: 'Benchmark'
    },
    width: {
      type: Number,
      default: 0
    },
    height: {
      type: Number,
      default: 500
    }
  },
  setup(props) {
    const chartContainer = ref(null)
    const currentData = ref(null)
    
    let chart = null
    let portfolioSeries = null
    let benchmarkSeries = null

    const createChartInstance = () => {
      if (!chartContainer.value) {
        console.error('Chart container not found')
        return
      }

      console.log('Creating chart with data:', {
        portfolioData: props.portfolioData?.length,
        benchmarkData: props.benchmarkData?.length,
        timestamps: props.timestamps?.length
      })

      chart = createChart(chartContainer.value, {
        autoSize: true,
        height: props.height,
        layout: {
          background: { type: ColorType.Solid, color: '#1e1e1e' },
          textColor: '#d1d4dc'
        },
        grid: {
          vertLines: { color: 'rgba(255, 255, 255, 0.1)' },
          horzLines: { color: 'rgba(255, 255, 255, 0.1)' }
        },
        crosshair: {
          mode: 1,
          vertLine: {
            width: 1,
            color: 'rgba(224, 227, 235, 0.5)',
            style: 0
          },
          horzLine: {
            width: 1,
            color: 'rgba(224, 227, 235, 0.5)',
            style: 0
          }
        },
        rightPriceScale: {
          borderColor: '#2b2b43',
          scaleMargins: {
            top: 0.1,
            bottom: 0.1
          }
        },
        timeScale: {
          borderColor: '#2b2b43',
          timeVisible: true,
          secondsVisible: false
        },
        handleScroll: {
          mouseWheel: true,
          pressedMouseMove: true,
          horzTouchDrag: true,
          vertTouchDrag: true
        },
        handleScale: {
          axisPressedMouseMove: true,
          mouseWheel: true,
          pinch: true
        }
      })

      portfolioSeries = chart.addSeries(LineSeries, {
        color: '#2196F3',
        lineWidth: 2,
        lastValueVisible: true,
        priceLineVisible: true,
        priceFormat: {
          type: 'custom',
          formatter: (price) => price.toFixed(2) + '%'
        }
      })

      benchmarkSeries = chart.addSeries(LineSeries, {
        color: '#FF9800',
        lineWidth: 2,
        lastValueVisible: true,
        priceLineVisible: true,
        priceFormat: {
          type: 'custom',
          formatter: (price) => price.toFixed(2) + '%'
        }
      })

      chart.subscribeCrosshairMove((param) => {
        if (!param || !param.time || param.point.x < 0 || param.point.y < 0) {
          currentData.value = null
          return
        }

        const portfolioPrice = param.seriesData.get(portfolioSeries)
        const benchmarkPrice = param.seriesData.get(benchmarkSeries)

        if (portfolioPrice && benchmarkPrice) {
          const date = new Date(param.time * 1000)
          const formattedDate = date.toLocaleDateString('en-US', { 
            year: 'numeric', 
            month: 'short', 
            day: 'numeric' 
          })

          const portfolioValue = portfolioPrice.value
          const benchmarkValue = benchmarkPrice.value
          const diff = portfolioValue - benchmarkValue

          currentData.value = {
            time: formattedDate,
            portfolio: portfolioValue.toFixed(2),
            benchmark: benchmarkValue.toFixed(2),
            diff: diff.toFixed(2)
          }
        }
      })

      updateChartData()
    }

    const updateChartData = () => {
      if (!portfolioSeries || !benchmarkSeries) {
        console.error('Series not initialized')
        return
      }

      if (!props.timestamps || !props.portfolioData || !props.benchmarkData) {
        console.error('Missing data')
        return
      }

      const portfolioData = props.timestamps.map((ts, index) => ({
        time: ts,
        value: props.portfolioData[index]
      }))

      const benchmarkData = props.timestamps.map((ts, index) => ({
        time: ts,
        value: props.benchmarkData[index]
      }))

      console.log('Setting data:', { 
        portfolioPoints: portfolioData.length,
        benchmarkPoints: benchmarkData.length,
        firstPortfolio: portfolioData[0],
        firstBenchmark: benchmarkData[0]
      })

      portfolioSeries.setData(portfolioData)
      benchmarkSeries.setData(benchmarkData)

      chart.timeScale().fitContent()
    }

    watch(() => [props.portfolioData, props.benchmarkData, props.timestamps], () => {
      if (chart) {
        console.log('Data changed, updating chart')
        updateChartData()
      }
    }, { deep: true })

    onMounted(() => {
      nextTick(() => {
        createChartInstance()
      })
    })

    onUnmounted(() => {
      if (chart) {
        chart.remove()
      }
    })

    return {
      chartContainer,
      currentData
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
  position: relative;
}

.chart-legend {
  position: absolute;
  top: 12px;
  left: 12px;
  background-color: rgba(30, 30, 30, 0.85);
  backdrop-filter: blur(10px);
  padding: 8px 12px;
  border-radius: 6px;
  display: flex;
  gap: 16px;
  font-size: 0.85rem;
  z-index: 10;
  border: 1px solid rgba(255, 255, 255, 0.1);
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.3);
}

.legend-item {
  color: #d1d4dc;
  display: flex;
  align-items: center;
  gap: 4px;
}

.font-weight-bold {
  font-weight: 600;
}

.text-primary {
  color: #2196F3;
}

.text-warning {
  color: #FF9800;
}

.text-success {
  color: #26a69a;
}

.text-error {
  color: #ef5350;
}
</style>
