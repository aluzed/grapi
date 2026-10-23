function offset(at: number, timeZone: string): number {
  const parts = Object.fromEntries(
    new Intl.DateTimeFormat('en-US', { timeZone, hourCycle: 'h23', year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' })
      .formatToParts(at)
      .map((p) => [p.type, Number(p.value)]),
  )
  return Date.UTC(parts.year!, parts.month! - 1, parts.day!, parts.hour!, parts.minute!, parts.second!) - Math.floor(at / 1000) * 1000
}

// The instant of a wall-clock time ("2026-11-02T09:30") in timeZone; the
// second pass settles the times next to a daylight saving change.
export function zonedToISO(local: string, timeZone: string): string {
  const [date, time = '00:00'] = local.split('T')
  const [y, m, d] = date!.split('-').map(Number)
  const [h, min] = time.split(':').map(Number)
  const wall = Date.UTC(y!, m! - 1, d!, h!, min!)
  let at = wall - offset(wall, timeZone)
  at = wall - offset(at, timeZone)
  return new Date(at).toISOString().replace('.000Z', 'Z')
}

// The wall-clock time of an instant in timeZone, for a datetime-local input.
export function isoToZoned(iso: string, timeZone: string): string {
  const at = Date.parse(iso)
  return new Date(at + offset(at, timeZone)).toISOString().slice(0, 16)
}

export function timeZones(): string[] {
  try {
    return Intl.supportedValuesOf('timeZone')
  } catch {
    return ['UTC']
  }
}

export const localTimeZone = () => Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC'
