import { gsap } from 'gsap'
import { ScrollTrigger } from 'gsap/ScrollTrigger'
import Lenis from 'lenis'

gsap.registerPlugin(ScrollTrigger)

let media: ReturnType<typeof gsap.matchMedia> | undefined

function cleanupMotion() {
  media?.revert()
  media = undefined
}

function setupMotion() {
  cleanupMotion()
  media = gsap.matchMedia()

  media.add('(prefers-reduced-motion: no-preference)', () => {
    const lenis = new Lenis({ autoRaf: false, anchors: true })
    const tick = (time: number) => lenis.raf(time * 1000)

    lenis.on('scroll', ScrollTrigger.update)
    gsap.ticker.add(tick)
    gsap.ticker.lagSmoothing(0)

    gsap.utils.toArray<HTMLElement>('[data-reveal]').forEach((element) => {
      gsap.from(element, {
        opacity: 0,
        y: 20,
        duration: 0.6,
        ease: 'power2.out',
        scrollTrigger: { trigger: element, start: 'top 90%', once: true },
      })
    })

    return () => {
      gsap.ticker.remove(tick)
      lenis.off('scroll', ScrollTrigger.update)
      lenis.destroy()
    }
  })
}

setupMotion()
document.addEventListener('astro:page-load', setupMotion)
document.addEventListener('astro:before-swap', cleanupMotion)
window.addEventListener('pagehide', cleanupMotion)
function restoreMotion(event: PageTransitionEvent) {
  if (event.persisted)
    setupMotion()
}

window.addEventListener('pageshow', restoreMotion)

if (import.meta.hot) {
  import.meta.hot.dispose(() => {
    cleanupMotion()
    document.removeEventListener('astro:page-load', setupMotion)
    document.removeEventListener('astro:before-swap', cleanupMotion)
    window.removeEventListener('pagehide', cleanupMotion)
    window.removeEventListener('pageshow', restoreMotion)
  })
}
