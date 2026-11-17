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

    gsap.from('#hero-title span', {
      y: 18,
      opacity: 0.4,
      duration: 0.75,
      stagger: 0.1,
      ease: 'power3.out',
    })

    gsap.utils.toArray<HTMLElement>('[data-reveal]').forEach((element) => {
      gsap.from(element, {
        opacity: 0.3,
        y: 18,
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

  media.add('(min-width: 1001px) and (prefers-reduced-motion: no-preference)', () => {
    gsap.from('[data-hero-image-frame]', {
      rotateX: 10,
      scale: 0.94,
      ease: 'none',
      scrollTrigger: {
        trigger: '[data-hero-figure]',
        start: 'top 95%',
        end: 'top 25%',
        scrub: 0.7,
      },
    })

    gsap.from('#features [data-screenshot-wrap]', {
      y: 32,
      scale: 0.97,
      ease: 'none',
      scrollTrigger: {
        trigger: '#features',
        start: 'top 85%',
        end: 'top 25%',
        scrub: 0.6,
      },
    })

    ScrollTrigger.create({
      trigger: '#pip-stage',
      start: 'center 65%',
      once: true,
      onEnter: () => document.dispatchEvent(new Event('mumu:show-pip')),
    })
  })

  const currentMedia = media
  void document.fonts.ready.then(() => {
    if (media === currentMedia)
      ScrollTrigger.refresh()
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
