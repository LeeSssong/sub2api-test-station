import type { Theme } from './themeSchedule'

export type SignalLayer = 'far' | 'mid' | 'near'

export interface SignalRowDescriptor {
  layer: SignalLayer
  seedIndex: number
  y: number
  speed: number
  alpha: number
  fontSize: number
  active: boolean
}

export interface SignalThemePalette {
  far: string
  mid: string
  near: string
  active: string
}

interface BuildSignalRowsOptions {
  width: number
  height: number
  random?: () => number
}

const layerOrder: SignalLayer[] = ['far', 'mid', 'near']
const layerTraits: Record<SignalLayer, Pick<SignalRowDescriptor, 'fontSize' | 'alpha' | 'speed'>> = {
  far: { fontSize: 11, alpha: .24, speed: -.34 },
  mid: { fontSize: 13, alpha: .46, speed: -.62 },
  near: { fontSize: 15, alpha: .7, speed: -.92 },
}

export function buildSignalRows({
  width,
  height,
  random = Math.random,
}: BuildSignalRowsOptions): SignalRowDescriptor[] {
  const count = Math.max(24, Math.ceil(height / 18) + (width >= 1200 ? 6 : 3))
  const rowGap = height / Math.max(1, count - 1)

  return Array.from({ length: count }, (_, index) => {
    const layer = layerOrder[index % layerOrder.length]!
    const traits = layerTraits[layer]
    const jitter = random()

    return {
      layer,
      seedIndex: index,
      y: index * rowGap,
      speed: traits.speed * (.82 + jitter * .36),
      alpha: traits.alpha * (.88 + jitter * .24),
      fontSize: traits.fontSize,
      active: index % 13 === 5,
    }
  })
}

export function signalPalette(theme: Theme): SignalThemePalette {
  return theme === 'light'
    ? {
        far: '#7890a8',
        mid: '#536f8c',
        near: '#294b6d',
        active: '#087b9b',
      }
    : {
        far: '#708299',
        mid: '#91a4bd',
        near: '#b9c9dc',
        active: '#4cc9e8',
      }
}
