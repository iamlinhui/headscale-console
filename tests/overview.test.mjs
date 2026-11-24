import { test } from 'node:test'
import assert from 'node:assert/strict'
import { recentDevices, activityLabel } from '../frontend/src/overview.js'
test('overview sorts online first then last-seen; does not mutate source order',()=>{
 const data=[{id:'a',name:'old-online',online:true,last:'2026-01-01T00:00:00Z'},{id:'b',name:'new-offline',online:false,last:'2026-11-24T00:00:00Z'},{id:'c',name:'recent-online',online:true,last:'2026-11-23T00:00:00Z'},{id:'d',name:'missing-online',online:true,last:null},{id:'e',name:'unknown',online:null,last:null}]
 assert.deepEqual(recentDevices(data).map(d=>d.id),['c','a','d','b'])
 assert.deepEqual(data.map(d=>d.id),['a','b','c','d','e'])
})
test('unknown activity is not invented and empty lists are supported',()=>{
 assert.equal(activityLabel({online:true,last:null},String),'当前在线')
 assert.equal(activityLabel({online:false,last:'bad'},String),'最后活跃时间未采集')
 assert.deepEqual(recentDevices([]),[])
})
