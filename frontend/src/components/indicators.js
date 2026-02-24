// Indicator engine: register indicator types and run them over OHLCV candle data.
//
// Each indicator definition:
//   id           - unique string key
//   label        - display name
//   color        - default line color (hex)
//   defaultParams - object with configurable fields (e.g. { period: 20 })
//   compute(candles, params) -> Array<{ time: number, value: number }>
//
// Usage:
//   import { register, compute, list } from './indicators.js'
//   list()                            // get all registered indicators
//   compute('MA', candles, { period: 50 })  // compute with custom params
//
// Extending:
//   register({
//     id: 'MY_IND', label: 'My Indicator', color: '#aabbcc',
//     defaultParams: { period: 14 },
//     compute(candles, { period }) { ... return [{ time, value }, ...] }
//   })

const registry = {}

function register(def) {
  registry[def.id] = def
}

function compute(id, candles, params) {
  const def = registry[id]
  if (!def) throw new Error(`Unknown indicator: ${id}`)
  return def.compute(candles, { ...def.defaultParams, ...params })
}

function list() {
  return Object.values(registry).map(({ id, label, color, defaultParams }) => ({ id, label, color, defaultParams }))
}

// Simple Moving Average
register({
  id: 'MA',
  label: 'MA',
  color: '#2196F3',
  defaultParams: { period: 20 },
  compute(candles, { period }) {
    const result = []
    for (let i = period - 1; i < candles.length; i++) {
      let sum = 0
      for (let j = i - period + 1; j <= i; j++) sum += candles[j].close
      result.push({ time: candles[i].time, value: sum / period })
    }
    return result
  }
})

// Exponential Moving Average
register({
  id: 'EMA',
  label: 'EMA',
  color: '#FF9800',
  defaultParams: { period: 20 },
  compute(candles, { period }) {
    if (candles.length < period) return []
    const k = 2 / (period + 1)
    let ema = 0
    for (let j = 0; j < period; j++) ema += candles[j].close
    ema /= period
    const result = [{ time: candles[period - 1].time, value: ema }]
    for (let i = period; i < candles.length; i++) {
      ema = candles[i].close * k + ema * (1 - k)
      result.push({ time: candles[i].time, value: ema })
    }
    return result
  }
})

// Volume Weighted Average Price (rolling window)
register({
  id: 'VWAP',
  label: 'VWAP',
  color: '#9C27B0',
  defaultParams: { period: 14 },
  compute(candles, { period }) {
    const result = []
    for (let i = period - 1; i < candles.length; i++) {
      let tpv = 0, vol = 0
      for (let j = i - period + 1; j <= i; j++) {
        const tp = (candles[j].high + candles[j].low + candles[j].close) / 3
        const v = candles[j].volume || 1
        tpv += tp * v
        vol += v
      }
      result.push({ time: candles[i].time, value: vol > 0 ? tpv / vol : 0 })
    }
    return result
  }
})

// Point of Control — price level of highest volume bar in the rolling window
register({
  id: 'POC',
  label: 'POC',
  color: '#F44336',
  defaultParams: { period: 20 },
  compute(candles, { period }) {
    const result = []
    for (let i = period - 1; i < candles.length; i++) {
      let maxVol = -1, pocPrice = 0
      for (let j = i - period + 1; j <= i; j++) {
        const v = candles[j].volume || 1
        if (v > maxVol) {
          maxVol = v
          pocPrice = (candles[j].high + candles[j].low + candles[j].close) / 3
        }
      }
      result.push({ time: candles[i].time, value: pocPrice })
    }
    return result
  }
})

// Time Price Opportunity Point of Control
// Divides the window's price range into buckets and counts how many candles
// touched each bucket (price range overlap). The bucket with the most touches
// is where price spent the most time — the TPO Point of Control.
register({
  id: 'TPO',
  label: 'TPO PoC',
  color: '#009688',
  defaultParams: { period: 20, levels: 50 },
  compute(candles, { period, levels }) {
    const numLevels = Math.max(10, levels || 50)
    const result = []

    for (let i = period - 1; i < candles.length; i++) {
      const window = candles.slice(i - period + 1, i + 1)

      let rangeHigh = -Infinity
      let rangeLow = Infinity
      for (const c of window) {
        if (c.high > rangeHigh) rangeHigh = c.high
        if (c.low < rangeLow) rangeLow = c.low
      }

      if (rangeHigh === rangeLow) {
        result.push({ time: candles[i].time, value: rangeHigh })
        continue
      }

      const bucketSize = (rangeHigh - rangeLow) / numLevels
      const counts = new Float64Array(numLevels)

      for (const c of window) {
        const firstBucket = Math.floor((c.low - rangeLow) / bucketSize)
        const lastBucket = Math.min(
          Math.floor((c.high - rangeLow) / bucketSize),
          numLevels - 1
        )
        for (let b = Math.max(0, firstBucket); b <= lastBucket; b++) {
          counts[b]++
        }
      }

      let maxCount = -1
      let pocBucket = 0
      for (let b = 0; b < numLevels; b++) {
        if (counts[b] > maxCount) {
          maxCount = counts[b]
          pocBucket = b
        }
      }

      const pocPrice = rangeLow + (pocBucket + 0.5) * bucketSize
      result.push({ time: candles[i].time, value: pocPrice })
    }

    return result
  }
})

export { register, compute, list }
