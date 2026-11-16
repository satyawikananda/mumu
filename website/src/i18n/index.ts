import { en } from './en'
import { id, ja, zh } from './translations'

export const languages = [
  { locale: 'en', name: 'English', path: '/' },
  { locale: 'id', name: 'Indonesia', path: '/id/' },
  { locale: 'zh-CN', name: '简体中文', path: '/zh-cn/' },
  { locale: 'ja', name: '日本語', path: '/ja/' },
] as const
export type Locale = (typeof languages)[number]['locale']
export const dictionaries = { en, id, 'zh-CN': zh, ja }
export type { Copy } from './en'
