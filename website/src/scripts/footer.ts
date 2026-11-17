import { gsap } from 'gsap'
import { ScrollTrigger } from 'gsap/ScrollTrigger'

gsap.registerPlugin(ScrollTrigger)

// Called inside the page's matchMedia context so every tween reverts with it.
export function setupFooterMotion() {
  const footer = document.querySelector<HTMLElement>('[data-footer]')
  if (!footer)
    return

  const select = gsap.utils.selector(footer)
  const buttons = [
    ...footer.querySelectorAll<HTMLButtonElement>('[data-footer-play]'),
  ]
  const events = new AbortController()
  const jumps = buttons.map(button => gsap.timeline({ paused: true })
    .to(button.parentElement, {
      y: -22,
      scaleX: 0.96,
      scaleY: 1.04,
      duration: 0.2,
      ease: 'power2.out',
    })
    .to(button.parentElement, {
      y: 0,
      scaleX: 1,
      scaleY: 1,
      duration: 0.4,
      ease: 'bounce.out',
    }))

  buttons.forEach((button, index) => {
    button.hidden = false
    button.disabled = false
    button.addEventListener('click', () => {
      if (!jumps[index].isActive())
        jumps[index].restart()
    }, { signal: events.signal })
  })

  if (!footer.hasAttribute('data-footer-revealed')) {
    gsap.timeline({
      scrollTrigger: {
        trigger: footer.querySelector('[data-footer-scene]'),
        start: 'top 85%',
        once: true,
        onEnter: () => footer.setAttribute('data-footer-revealed', ''),
      },
    })
      .from(select('[data-footer-ground]'), { scaleX: 0, duration: 0.35 }, 0)
      .from(select('[data-footer-letter]'), {
        y: -22,
        opacity: 0,
        scale: 1.06,
        stagger: 0.08,
        duration: 0.45,
        ease: 'back.out(1.5)',
      }, 0.15)
      .from(select('[data-footer-crew]'), {
        y: 28,
        opacity: 0,
        stagger: 0.1,
        duration: 0.45,
        ease: 'back.out(1.8)',
      }, 0.55)
      .from(select('[data-footer-doodle]'), {
        scale: 0.6,
        opacity: 0,
        stagger: 0.04,
        duration: 0.3,
      }, 0.9)
  }

  return () => {
    events.abort()
    buttons.forEach((button) => {
      button.hidden = true
      button.disabled = true
    })
  }
}
