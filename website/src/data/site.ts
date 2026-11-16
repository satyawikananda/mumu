export const repository = 'https://github.com/satyawikananda/mumu'

// Fill these only after the public DMG and the approved recording are available.
export const release: { macDownloadUrl: string, version: string } = {
  macDownloadUrl: '',
  version: '',
}
export const demoVideo: { src: string } = { src: '' }

export function isDownloadReady(url: string): boolean {
  try {
    const parsed = new URL(url)
    return parsed.protocol === 'https:' && parsed.pathname.endsWith('.dmg')
  }
  catch {
    return false
  }
}
