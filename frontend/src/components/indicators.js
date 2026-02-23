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

// Time Price Opportunity midpoint — average of typical prices in the rolling window
register({
  id: 'TPO',
  label: 'TPO Mid',
  color: '#009688',
  defaultParams: { period: 20 },
  compute(candles, { period }) {
    const result = []
    for (let i = period - 1; i < candles.length; i++) {
      let sum = 0
      for (let j = i - period + 1; j <= i; j++) {
        sum += (candles[j].high + candles[j].low + candles[j].close) / 3
      }
      result.push({ time: candles[i].time, value: sum / period })
    }
    return result
  }
})

export { register, compute, list }
