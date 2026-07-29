import { existsSync, readFileSync, statSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const projectRoot = resolve(import.meta.dirname, '../..')
const publishedLogo = resolve(projectRoot, 'public/home-assets/xingqiao-logo-256-v1.webp')
const legacyLogo = resolve(projectRoot, 'public/home-assets/xingqiao-logo.png')
const sourceMaster = resolve(projectRoot, 'assets/source/xingqiao-logo-master.png')

function readWebpDimensions(buffer: Buffer): { width: number; height: number } {
  if (buffer.toString('ascii', 0, 4) !== 'RIFF' || buffer.toString('ascii', 8, 12) !== 'WEBP') {
    throw new Error('not a WebP RIFF file')
  }

  const chunk = buffer.toString('ascii', 12, 16)
  if (chunk === 'VP8X') {
    return {
      width: 1 + buffer.readUIntLE(24, 3),
      height: 1 + buffer.readUIntLE(27, 3),
    }
  }
  if (chunk === 'VP8 ') {
    return {
      width: buffer.readUInt16LE(26) & 0x3fff,
      height: buffer.readUInt16LE(28) & 0x3fff,
    }
  }
  if (chunk === 'VP8L') {
    const bits = buffer.readUInt32LE(21)
    return {
      width: 1 + (bits & 0x3fff),
      height: 1 + ((bits >> 14) & 0x3fff),
    }
  }
  throw new Error(`unsupported WebP chunk ${chunk}`)
}

describe('published Xingqiao logo', () => {
  it('ships only a 256px WebP no larger than 50 KiB', () => {
    expect(existsSync(sourceMaster)).toBe(true)
    expect(existsSync(legacyLogo)).toBe(false)
    expect(existsSync(publishedLogo)).toBe(true)

    const dimensions = readWebpDimensions(readFileSync(publishedLogo))
    expect(dimensions).toEqual({ width: 256, height: 256 })
    expect(statSync(publishedLogo).size).toBeLessThanOrEqual(51_200)
  })
})
