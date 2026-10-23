// Box is a selection in fractions of the displayed image (0 to 1).
export type Box = { x: number; y: number; w: number; h: number }

// cropRect turns a selection into whole source pixels, inside the image.
export function cropRect(box: Box, width: number, height: number) {
  const x = Math.min(Math.max(Math.round(box.x * width), 0), width - 1)
  const y = Math.min(Math.max(Math.round(box.y * height), 0), height - 1)
  return {
    x,
    y,
    width: Math.max(1, Math.min(Math.round(box.w * width), width - x)),
    height: Math.max(1, Math.min(Math.round(box.h * height), height - y)),
  }
}
