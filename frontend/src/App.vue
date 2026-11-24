<script setup>
import { zhCN, dateZhCN, NConfigProvider, NMessageProvider, NDialogProvider } from 'naive-ui'
import Console from './Console.vue'
const theme = {
  common: {
    primaryColor: '#2457F5', primaryColorHover: '#1746D9', primaryColorPressed: '#143ABD', primaryColorSuppl: '#2457F5',
    textColorBase: '#111318', textColor1: '#111318', textColor2: '#414651', textColor3: '#626974',
    borderColor: '#DCE0E6', dividerColor: '#E2E5EA', bodyColor: '#FFFFFF', cardColor: '#FFFFFF',
    fontFamily: 'Inter, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif',
    fontSize: '15px', borderRadius: '7px', heightMedium: '40px'
  },
  Button: { fontWeight: '550', borderRadiusMedium: '7px' },
  Menu: { itemTextColorActive: '#2457F5', itemIconColorActive: '#2457F5', itemColorActive: '#EDF2FF', itemColorActiveHover: '#E6EDFF', itemHeight: '43px', borderRadius: '7px' },
  DataTable: { thColor: '#F8F9FB', thTextColor: '#626974', tdTextColor: '#414651', tdColorHover: '#F8FAFF', borderColor: '#E2E5EA', thFontWeight: '500' },
  Tabs: { tabFontWeightActive: '600' },
  Tag: { borderRadius: '4px' }
}
</script>
<template>
  <n-config-provider :theme-overrides="theme" :locale="zhCN" :date-locale="dateZhCN">
    <n-message-provider><n-dialog-provider><Console /></n-dialog-provider></n-message-provider>
  </n-config-provider>
</template>
