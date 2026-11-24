// Control-plane activity only; never infer traffic or service reachability.
export function activityTime(device) {
  const time = typeof device.last === 'string' ? Date.parse(device.last) : NaN
  return Number.isFinite(time) ? time : null
}
export function recentDevices(devices, limit = 4) {
  return [...devices].sort((a, b) => {
    const rank = d => d.online === true ? 2 : d.online === false ? 1 : 0
    const status = rank(b) - rank(a)
    if (status) return status
    const at = activityTime(a), bt = activityTime(b)
    if (at !== bt) return (bt ?? -Infinity) - (at ?? -Infinity)
    return String(a.name).localeCompare(String(b.name), 'zh-CN')
  }).slice(0, limit)
}
export function activityLabel(device, formatDate) {
  if (device.online === true) return '当前在线'
  if (activityTime(device) !== null) return formatDate(device.last)
  return '最后活跃时间未采集'
}
