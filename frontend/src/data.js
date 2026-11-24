export const demoDevices = [
  { id: 1, name: 'prod-gateway-01', os: 'Ubuntu 24.04', platform: 'Linux', ip: '100.64.0.1', owner: 'Infrastructure', online: true, tags: ['prod', 'router'], expiry: '永不过期', last: '刚刚', version: '1.80.2', routes: ['10.0.0.0/24', '192.168.10.0/24'] },
  { id: 2, name: 'jason-macbook', os: 'macOS Sequoia', platform: 'macOS', ip: '100.64.0.12', owner: 'Jason Deng', online: true, tags: [], expiry: '2026-12-01', last: '刚刚', version: '1.80.2', routes: [] },
  { id: 3, name: 'dev-server-02', os: 'Debian 12', platform: 'Linux', ip: '100.64.0.8', owner: 'Infrastructure', online: true, tags: ['dev'], expiry: '永不过期', last: '刚刚', version: '1.78.3', routes: [] },
  { id: 4, name: 'anna-thinkpad', os: 'Windows 11', platform: 'Windows', ip: '100.64.0.15', owner: 'Anna Chen', online: true, tags: [], expiry: '2027-02-18', last: '刚刚', version: '1.80.2', routes: [] },
  { id: 5, name: 'office-router', os: 'OpenWrt', platform: 'Linux', ip: '100.64.0.3', owner: 'Infrastructure', online: true, tags: ['office', 'router'], expiry: '永不过期', last: '刚刚', version: '1.80.2', routes: ['172.16.0.0/16'] },
  { id: 6, name: 'jason-iphone', os: 'iOS 18.2', platform: 'iOS', ip: '100.64.0.19', owner: 'Jason Deng', online: true, tags: [], expiry: '2027-01-20', last: '刚刚', version: '1.80.0', routes: [] },
  { id: 7, name: 'build-runner-01', os: 'Ubuntu 22.04', platform: 'Linux', ip: '100.64.0.21', owner: 'Infrastructure', online: false, tags: ['ci'], expiry: '2026-11-27', last: '2 小时前', version: '1.78.3', routes: [] },
  { id: 8, name: 'test-laptop', os: 'Windows 11', platform: 'Windows', ip: '100.64.0.24', owner: 'Leo Wang', online: false, tags: ['test'], expiry: '已过期', last: '3 天前', version: '1.76.6', routes: [] }
]
export const demoUsers = [
  { id: 1, name: 'Jason Deng', email: 'jason@acme.internal', initial: 'JD' },
  { id: 2, name: 'Anna Chen', email: 'anna@acme.internal', initial: 'AC' },
  { id: 3, name: 'Leo Wang', email: 'leo@acme.internal', initial: 'LW' },
  { id: 4, name: 'Infrastructure', email: 'infra@acme.internal', initial: 'IF' }
]
export const demoRoutes = [
  { id: 1, cidr: '10.0.0.0/24', nodeId: 1, description: '生产环境子网', approved: true },
  { id: 2, cidr: '192.168.10.0/24', nodeId: 1, description: '数据中心子网', approved: true },
  { id: 3, cidr: '172.16.0.0/16', nodeId: 5, description: '办公室网络', approved: false }
]
export const demoKeys = [
  { id: 1, name: '生产服务器接入', prefix: '•••• 8f2a', owner: 'Infrastructure', reusable: true, expiry: '2026-12-24', status: '有效' },
  { id: 2, name: '个人设备接入', prefix: '•••• 1c9b', owner: 'Jason Deng', reusable: false, expiry: '2026-11-26', status: '有效' },
  { id: 3, name: 'CI 临时节点', prefix: '•••• 7d4e', owner: 'Infrastructure', reusable: true, expiry: '2026-11-20', status: '已过期' }
]
export const initialPolicy = JSON.stringify({
  groups: { 'group:engineering': ['jason@', 'anna@'] },
  tagOwners: { 'tag:prod': ['group:engineering'], 'tag:dev': ['group:engineering'] },
  grants: [
    { src: ['group:engineering'], dst: ['tag:dev'], ip: ['*'] },
    { src: ['jason@'], dst: ['tag:prod'], ip: ['tcp:22', 'tcp:443'] }
  ]
}, null, 2)
