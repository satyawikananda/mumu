document.documentElement.classList.add('js')

try {
  const preference = localStorage.getItem('mumu-theme') || 'system'
  const explicitDark = preference === 'dark'
  const systemDark = preference === 'system' && matchMedia('(prefers-color-scheme: dark)').matches
  document.documentElement.dataset.theme = explicitDark || systemDark
    ? 'dark'
    : 'light'
}
catch {
  // CSS follows the system when storage is unavailable.
}
