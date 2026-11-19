export const repository = 'https://github.com/satyawikananda/mumu'
export const VERSION = 'v0.1.0'

// Fill these only after the public DMG and the approved recording are available.
export const release: { macDownloadUrl: string, version: string } = {
  macDownloadUrl: `${repository}/releases/tag/${VERSION}`,
  version: VERSION,
}
export const demoVideo: { src: string } = { src: '' }

export function isDownloadReady(url: string): boolean {
  try {
    const parsed = new URL(url)
    return (
      parsed.protocol === 'https:'
      && parsed.hostname === 'github.com'
      && parsed.pathname.startsWith('/satyawikananda/mumu/releases/tag/')
    )
  }
  catch {
    return false
  }
}
