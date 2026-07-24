export interface SignalTile {
  text: string
  segmentWidth: number
  repetitions: number
  coveredWidth: number
}

export function buildSignalTile(
  segment: string,
  segmentWidth: number,
  viewportWidth: number,
): SignalTile {
  const safeSegmentWidth = Math.max(1, segmentWidth)
  const safeViewportWidth = Math.max(1, viewportWidth)
  const repetitions = Math.max(2, Math.ceil(safeViewportWidth / safeSegmentWidth) + 2)

  return {
    text: segment.repeat(repetitions),
    segmentWidth: safeSegmentWidth,
    repetitions,
    coveredWidth: repetitions * safeSegmentWidth,
  }
}
