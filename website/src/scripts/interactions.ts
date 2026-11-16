const lifecycle = new AbortController()
const { signal } = lifecycle
document.documentElement.classList.add('js')

function storePreference(key: string, value: string) {
  try {
    localStorage.setItem(key, value)
  }
  catch { /* All controls still work with storage disabled. */ }
}

const root = document.documentElement
const themeOptions = [
  ...document.querySelectorAll<HTMLInputElement>('[data-theme-option]'),
]
const systemTheme = matchMedia('(prefers-color-scheme: dark)')
let themePreference = 'system'
try {
  const saved = localStorage.getItem('mumu-theme')
  if (saved && ['light', 'dark', 'system'].includes(saved))
    themePreference = saved
}
catch { /* Follow the system when storage is unavailable. */ }
function applyTheme() {
  root.dataset.theme = themePreference === 'dark' || (themePreference === 'system' && systemTheme.matches)
    ? 'dark'
    : 'light'
  themeOptions.forEach(input => input.checked = input.value === themePreference)
}
themeOptions.forEach(input => input.addEventListener('change', () => {
  if (!input.checked)
    return
  themePreference = input.value
  storePreference('mumu-theme', themePreference)
  applyTheme()
}, { signal }))
systemTheme.addEventListener('change', applyTheme, { signal })
applyTheme()

const header = document.querySelector<HTMLElement>('.site-header')
const sentinel = document.querySelector<HTMLElement>('.navigation-sentinel')
const headerObserver = new IntersectionObserver(([entry]) => {
  if (entry)
    header?.classList.toggle('is-compact', entry.boundingClientRect.top < 0)
})
if (sentinel) {
  header?.classList.toggle('is-compact', sentinel.getBoundingClientRect().top < 0)
  headerObserver.observe(sentinel)
}

const languagePickers = [
  ...document.querySelectorAll<HTMLDetailsElement>('.language-picker'),
]
document.querySelectorAll<HTMLAnchorElement>('[data-language]').forEach((anchor) => {
  anchor.addEventListener('click', () => {
    storePreference('mumu-language', anchor.dataset.language!)
    const destination = new URL(anchor.href)
    destination.hash = window.location.hash
    anchor.href = destination.href
  }, { signal })
})
for (const picker of languagePickers) {
  const summary = picker.querySelector('summary')!
  const links = [
    ...picker.querySelectorAll<HTMLAnchorElement>('a'),
  ]
  picker.addEventListener('toggle', () => {
    if (picker.open)
      languagePickers.filter(other => other !== picker).forEach(other => other.open = false)
  }, { signal })
  picker.addEventListener('keydown', (event) => {
    const index = links.indexOf(document.activeElement as HTMLAnchorElement)
    let next: number
    if (event.key === 'ArrowDown') {
      next = (index + 1) % links.length
    }
    else if (event.key === 'ArrowUp') {
      next = index < 0
        ? links.length - 1
        : (index + links.length - 1) % links.length
    }
    else if (event.key === 'Home') {
      next = 0
    }
    else if (event.key === 'End') {
      next = links.length - 1
    }
    else {
      return
    }
    event.preventDefault()
    picker.open = true
    links[next]?.focus()
  }, { signal })
  picker.addEventListener('focusout', (event) => {
    if (event.relatedTarget instanceof Node && !picker.contains(event.relatedTarget))
      picker.open = false
  }, { signal })
  summary.addEventListener('click', () => {
    languagePickers.filter(other => other !== picker).forEach(other => other.open = false)
  }, { signal })
}
document.addEventListener('click', (event) => {
  const target = event.target
  if (target instanceof Node)
    languagePickers.filter(picker => !picker.contains(target)).forEach(picker => picker.open = false)
}, { signal })
document.addEventListener('keydown', (event) => {
  if (event.key !== 'Escape')
    return
  const picker = languagePickers.find(picker => picker.open)
  if (picker) {
    event.preventDefault()
    picker.open = false
    picker.querySelector('summary')?.focus()
  }
}, { signal })

const tabs = [
  ...document.querySelectorAll<HTMLButtonElement>('[data-screen]'),
]
function selectTab(tab: HTMLButtonElement, focus = false) {
  for (const item of tabs) {
    const selected = item === tab
    item.setAttribute('aria-selected', String(selected))
    item.tabIndex = selected
      ? 0
      : -1
    const panel = document.getElementById(`panel-${item.dataset.screen}`)
    if (panel)
      panel.hidden = !selected
  }
  if (focus)
    tab.focus()
  const panel = document.getElementById(`panel-${tab.dataset.screen}`)
  if (!matchMedia('(prefers-reduced-motion: reduce)').matches) {
    panel?.animate([
      { opacity: 0.5, transform: 'translateY(8px)' },
      { opacity: 1, transform: 'translateY(0)' },
    ], { duration: 250, easing: 'ease-out' })
  }
}
for (const [index, tab] of tabs.entries()) {
  tab.addEventListener('click', () => selectTab(tab), { signal })
  tab.addEventListener('keydown', (event) => {
    let next = index
    if (event.key === 'ArrowRight')
      next = (index + 1) % tabs.length
    else if (event.key === 'ArrowLeft')
      next = (index + tabs.length - 1) % tabs.length
    else if (event.key === 'Home')
      next = 0
    else if (event.key === 'End')
      next = tabs.length - 1
    else return
    event.preventDefault()
    selectTab(tabs[next]!, true)
  }, { signal })
}

const dialog = document.querySelector<HTMLDialogElement>('.screenshot-dialog')
const dialogImage = dialog?.querySelector<HTMLImageElement>('.dialog-image')
let expandedBy: HTMLButtonElement | undefined
document.querySelectorAll<HTMLButtonElement>('[data-expand]').forEach((button) => {
  button.addEventListener('click', () => {
    const source = document.querySelector<HTMLImageElement>(`#panel-${button.dataset.expand} img`)
    if (!source || !dialog || !dialogImage)
      return
    dialogImage.src = source.currentSrc || source.src
    dialogImage.alt = source.alt
    dialogImage.width = source.naturalWidth || source.width
    dialogImage.height = source.naturalHeight || source.height
    expandedBy = button
    dialog.showModal()
    document.documentElement.style.overflow = 'hidden'
  }, { signal })
})
dialog?.querySelector('.dialog-close')?.addEventListener('click', () => dialog.close(), { signal })
dialog?.addEventListener('click', (event) => {
  if (event.target === dialog)
    dialog.close()
}, { signal })
dialog?.addEventListener('close', () => {
  document.documentElement.style.overflow = ''
  expandedBy?.focus()
}, { signal })

const favorite = document.querySelector<HTMLButtonElement>('#favorite-toggle')
favorite?.addEventListener('click', () => {
  const saved = favorite.getAttribute('aria-pressed') !== 'true'
  favorite.setAttribute('aria-pressed', String(saved))
  favorite.setAttribute('aria-label', (saved ? favorite.dataset.remove : favorite.dataset.add) || '')
  const status = document.querySelector<HTMLElement>('.favorite-status')
  if (status)
    status.textContent = (saved ? status.dataset.saved : status.dataset.empty) || ''
}, { signal })

const stage = document.querySelector<HTMLElement>('#pip-stage')
const player = stage?.querySelector<HTMLElement>('.mini-player')
const pipToggle = document.querySelector<HTMLButtonElement>('#pip-toggle')
const cornerButtons = [
  ...document.querySelectorAll<HTMLButtonElement>('[data-corner-button]'),
]
let floating = false
let manualPip = false
let corner = 3
let drag: { id: number, x: number, y: number, left: number, top: number } | undefined

function positionPlayer(left?: number, top?: number) {
  if (!player || !stage)
    return
  const width = stage.clientWidth * 0.48
  const height = width * 1840 / 2940
  const inset = 12
  const maxX = Math.max(inset, stage.clientWidth - width - inset)
  const maxY = Math.max(inset, stage.clientHeight - height - inset)
  const x = Math.min(maxX, Math.max(inset, left ?? (corner % 2
    ? maxX
    : inset)))
  const y = Math.min(maxY, Math.max(inset, top ?? (corner >= 2
    ? maxY
    : inset)))
  player.style.transform = floating
    ? `translate(${x}px, ${y}px)`
    : 'translate(0, 0)'
}
function setFloating(value: boolean) {
  floating = value
  stage?.classList.toggle('is-floating', value)
  pipToggle?.setAttribute('aria-pressed', String(value))
  const label = pipToggle?.querySelector('span')
  if (label && pipToggle)
    label.textContent = (value ? pipToggle.dataset.exit : pipToggle.dataset.enter) || ''
  if (player) {
    player.tabIndex = value
      ? 0
      : -1
  }
  positionPlayer()
}
function setCorner(value: number) {
  manualPip = true
  corner = value
  cornerButtons.forEach((button, index) => button.setAttribute('aria-pressed', String(index === value)))
  setFloating(true)
}
pipToggle?.addEventListener('click', () => {
  manualPip = true
  setFloating(!floating)
}, { signal })
document.addEventListener('mumu:show-pip', () => {
  if (!manualPip)
    setFloating(true)
}, { signal })
cornerButtons.forEach(button => button.addEventListener('click', () => setCorner(Number(button.dataset.cornerButton)), { signal }))
player?.addEventListener('keydown', (event) => {
  if (!floating)
    return
  const changes: Record<string, number> = {
    ArrowLeft: corner >= 2
      ? 2
      : 0,
    ArrowRight: corner >= 2
      ? 3
      : 1,
    ArrowUp: corner % 2,
    ArrowDown: 2 + corner % 2,
  }
  if (changes[event.key] !== undefined) {
    event.preventDefault()
    setCorner(changes[event.key]!)
  }
}, { signal })
player?.addEventListener('pointerdown', (event) => {
  if (!floating || !stage || event.button !== 0)
    return
  manualPip = true
  const rect = player.getBoundingClientRect()
  const bounds = stage.getBoundingClientRect()
  drag = {
    id: event.pointerId,
    x: event.clientX,
    y: event.clientY,
    left: rect.left - bounds.left,
    top: rect.top - bounds.top,
  }
  player.setPointerCapture(event.pointerId)
  player.classList.add('is-dragging')
}, { signal })
player?.addEventListener('pointermove', (event) => {
  if (drag?.id !== event.pointerId)
    return
  positionPlayer(drag.left + event.clientX - drag.x, drag.top + event.clientY - drag.y)
  cornerButtons.forEach(button => button.setAttribute('aria-pressed', 'false'))
}, { signal })
function finishDrag() {
  drag = undefined
  player?.classList.remove('is-dragging')
}
player?.addEventListener('pointerup', finishDrag, { signal })
player?.addEventListener('pointercancel', finishDrag, { signal })
player?.addEventListener('lostpointercapture', finishDrag, { signal })
const resizeObserver = new ResizeObserver(() => positionPlayer())
if (stage)
  resizeObserver.observe(stage)
setFloating(false)

document.querySelector<HTMLVideoElement>('video')?.addEventListener('error', () => {
  const error = document.querySelector<HTMLElement>('.video-error')
  if (error)
    error.hidden = false
}, { signal })
document.querySelector<HTMLSourceElement>('video source')?.addEventListener('error', () => {
  const error = document.querySelector<HTMLElement>('.video-error')
  if (error)
    error.hidden = false
}, { signal })

if (import.meta.hot) {
  import.meta.hot.dispose(() => {
    lifecycle.abort()
    headerObserver.disconnect()
    resizeObserver.disconnect()
  })
}
