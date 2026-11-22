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

// Bollinger Bands: middle is the simple average, bands are a standard deviation away.
function bollinger(candles, period, mult) {
  const upper = [], middle = [], lower = []
  for (let i = period - 1; i < candles.length; i++) {
    let sum = 0
    for (let j = i - period + 1; j <= i; j++) sum += candles[j].close
    const mean = sum / period
    let variance = 0
    for (let j = i - period + 1; j <= i; j++) variance += (candles[j].close - mean) ** 2
    const deviation = Math.sqrt(variance / period)
    upper.push({ time: candles[i].time, value: mean + mult * deviation })
    middle.push({ time: candles[i].time, value: mean })
    lower.push({ time: candles[i].time, value: mean - mult * deviation })
  }
  return { upper, middle, lower }
}

register({
  id: 'BOLL_UP',
  label: 'Bollinger upper',
  color: '#5c6bc0',
  defaultParams: { period: 20, mult: 2 },
  compute(candles, params) { return bollinger(candles, params.period, params.mult).upper }
})

register({
  id: 'BOLL_LOW',
  label: 'Bollinger lower',
  color: '#5c6bc0',
  defaultParams: { period: 20, mult: 2 },
  compute(candles, params) { return bollinger(candles, params.period, params.mult).lower }
})

// Donchian channel: highest high and lowest low of the window.
register({
  id: 'DON_UP',
  label: 'Donchian high',
  color: '#78909c',
  defaultParams: { period: 20 },
  compute(candles, { period }) {
    const result = []
    for (let i = period - 1; i < candles.length; i++) {
      let high = -Infinity
      for (let j = i - period + 1; j <= i; j++) high = Math.max(high, candles[j].high)
      result.push({ time: candles[i].time, value: high })
    }
    return result
  }
})

register({
  id: 'DON_LOW',
  label: 'Donchian low',
  color: '#78909c',
  defaultParams: { period: 20 },
  compute(candles, { period }) {
    const result = []
    for (let i = period - 1; i < candles.length; i++) {
      let low = Infinity
      for (let j = i - period + 1; j <= i; j++) low = Math.min(low, candles[j].low)
      result.push({ time: candles[i].time, value: low })
    }
    return result
  }
})

// Volume profile over the loaded window: the price levels where the most volume
// traded, plus the 70 percent value area around the point of control.
function volumeProfile(candles, levels = 40) {
  if (!candles.length) return null
  let high = -Infinity
  let low = Infinity
  for (const candle of candles) {
    high = Math.max(high, candle.high)
    low = Math.min(low, candle.low)
  }
  if (!Number.isFinite(high) || !Number.isFinite(low) || high <= low) return null
  const buckets = new Float64Array(levels)
  const step = (high - low) / levels
  for (const candle of candles) {
    const volume = candle.volume || 1
    const first = Math.max(0, Math.floor((candle.low - low) / step))
    const last = Math.min(levels - 1, Math.floor((candle.high - low) / step))
    const span = last - first + 1
    for (let i = first; i <= last; i++) buckets[i] += volume / span
  }
  let pocIndex = 0
  let total = 0
  for (let i = 0; i < levels; i++) {
    total += buckets[i]
    if (buckets[i] > buckets[pocIndex]) pocIndex = i
  }
  let lower = pocIndex
  let upper = pocIndex
  let covered = buckets[pocIndex]
  while (covered < total * 0.7 && (lower > 0 || upper < levels - 1)) {
    const below = lower > 0 ? buckets[lower - 1] : -1
    const above = upper < levels - 1 ? buckets[upper + 1] : -1
    if (above >= below) {
      upper++
      covered += Math.max(above, 0)
    } else {
      lower--
      covered += Math.max(below, 0)
    }
  }
  const price = (index) => low + (index + 0.5) * step
  return { poc: price(pocIndex), vah: price(upper), val: price(lower) }
}

export { register, compute, list, volumeProfile }

