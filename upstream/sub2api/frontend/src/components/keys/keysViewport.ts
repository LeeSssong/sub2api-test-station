/** Keep full rows within the available inventory, using the measured scrollport (clientHeight excludes its scrollbar). */
export function fitKeyRows(height: number, headerHeight = 44, rowHeight = 64): number {
  return Math.max(1, Math.floor((height - headerHeight - 1) / Math.max(1, rowHeight)))
}

/** Line menus always start below their trigger; the list scrolls within the remaining space. */
export function placeKeyMenu(rect: { left: number; bottom: number }, viewportWidth: number, viewportHeight: number) {
  const width = Math.min(420, viewportWidth - 16)
  const top = rect.bottom + 4
  return {
    top,
    left: Math.max(8, Math.min(rect.left, viewportWidth - width - 8)),
    maxHeight: Math.max(0, Math.min(400, viewportHeight - top - 8)),
    width,
  }
}
