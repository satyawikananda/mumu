// Shader adapted from the Cloudscape component supplied from 21st.dev.
const vertexShaderGLSL = `
attribute vec2 position;
void main() {
  gl_Position = vec4(position, 0.0, 1.0);
}
`

const fragmentShaderGLSL = `
precision highp float;

uniform vec2 u_resolution;
uniform float u_time;
uniform vec3 u_colorBottom;
uniform vec3 u_colorMid;
uniform vec3 u_colorTop;
uniform float u_speed;

float hash(vec2 p) {
  return fract(sin(dot(p, vec2(127.1, 311.7))) * 43758.5453123);
}

float noise(vec2 p) {
  vec2 i = floor(p);
  vec2 f = fract(p);
  float a = hash(i);
  float b = hash(i + vec2(1.0, 0.0));
  float c = hash(i + vec2(0.0, 1.0));
  float d = hash(i + vec2(1.0, 1.0));
  vec2 u = f * f * (3.0 - 2.0 * f);
  return mix(a, b, u.x) + (c - a) * u.y * (1.0 - u.x) + (d - b) * u.x * u.y;
}

float fbm(vec2 p, float t) {
  float v = 0.0;
  float a = 0.5;
  float fi = 0.0;
  mat2 rot = mat2(0.86, 0.51, -0.51, 0.86);
  
  for (int i = 0; i < 6; i++) {
    vec2 morph = vec2(sin(t * 0.5 + fi), cos(t * 0.3 - fi)) * 0.05;
    v += a * noise(p + morph);
    p = rot * p * 2.0;
    a *= 0.5;
    fi += 1.0;
  }
  return v;
}

void main() {
  vec2 uv = gl_FragCoord.xy / u_resolution;
  float t = u_time * u_speed;
  vec2 aspect = vec2(u_resolution.x / max(u_resolution.y, 1.0), 1.0);
  vec2 p = (uv - 0.5) * aspect;

  vec2 wind = vec2(t * 0.1, t * 0.02);

  float pattern = fbm(p * 4.5 + vec2(3.7, 1.3) - wind, t);

  float bandLow = smoothstep(0.32, 0.62, pattern);
  float bandHigh = smoothstep(0.58, 0.78, pattern); 
  
  vec3 color = mix(u_colorBottom, u_colorMid, bandLow);
  color = mix(color, u_colorTop, bandHigh);

  gl_FragColor = vec4(color, 1.0);
}
`

interface Renderer {
  draw: (time: number) => void
  resize: () => void
  updateColors: () => void
  destroy: () => void
}

function createRenderer(host: HTMLElement, canvas: HTMLCanvasElement): Renderer | undefined {
  const gl = canvas.getContext('webgl', { alpha: false, antialias: false, depth: false })
  if (!gl)
    return

  const shaders: WebGLShader[] = []
  const program = gl.createProgram()
  const buffer = gl.createBuffer()
  function destroy() {
    delete host.dataset.ready
    gl!.deleteBuffer(buffer)
    gl!.deleteProgram(program)
    shaders.forEach(shader => gl!.deleteShader(shader))
  }
  function compile(type: number, source: string) {
    const shader = gl!.createShader(type)
    if (!shader)
      throw new Error('Unable to allocate cloud shader')
    shaders.push(shader)
    gl!.shaderSource(shader, source)
    gl!.compileShader(shader)
    if (!gl!.getShaderParameter(shader, gl!.COMPILE_STATUS))
      throw new Error('Unable to compile cloud shader')
    return shader
  }

  try {
    if (!program || !buffer)
      throw new Error('Unable to allocate cloud renderer')
    gl.attachShader(program, compile(gl.VERTEX_SHADER, vertexShaderGLSL))
    gl.attachShader(program, compile(gl.FRAGMENT_SHADER, fragmentShaderGLSL))
    gl.linkProgram(program)
    if (!gl.getProgramParameter(program, gl.LINK_STATUS))
      throw new Error('Unable to link cloud shader')
    gl.useProgram(program)
    const position = gl.getAttribLocation(program, 'position')
    if (position < 0)
      throw new Error('Missing cloud position attribute')
    gl.bindBuffer(gl.ARRAY_BUFFER, buffer)
    gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([-1, -1, 1, -1, -1, 1, 1, 1]), gl.STATIC_DRAW)
    gl.enableVertexAttribArray(position)
    gl.vertexAttribPointer(position, 2, gl.FLOAT, false, 0, 0)
    const resolution = gl.getUniformLocation(program, 'u_resolution')
    const time = gl.getUniformLocation(program, 'u_time')
    const speed = Number(host.dataset.speed)
    gl.uniform1f(gl.getUniformLocation(program, 'u_speed'), Number.isFinite(speed)
      ? speed
      : 0.35)

    function resize() {
      const { width, height } = host.getBoundingClientRect()
      const scale = Math.min(window.devicePixelRatio || 1, 1.5, Math.sqrt(2_000_000 / Math.max(1, width * height)))
      canvas.width = Math.max(1, Math.floor(width * scale))
      canvas.height = Math.max(1, Math.floor(height * scale))
      gl!.viewport(0, 0, canvas.width, canvas.height)
      gl!.uniform2f(resolution, canvas.width, canvas.height)
    }
    function draw(elapsed: number) {
      gl!.uniform1f(time, elapsed)
      gl!.drawArrays(gl!.TRIANGLE_STRIP, 0, 4)
      host.dataset.ready = ''
    }
    function updateColors() {
      const style = getComputedStyle(host)
      for (const [token, uniform] of [
        ['bottom', 'Bottom'],
        ['mid', 'Mid'],
        ['top', 'Top'],
      ]) {
        const hex = style.getPropertyValue(`--cloud-${token}`).trim().replace('#', '')
        const rgb = [0, 2, 4].map(offset => Number.parseInt(hex.slice(offset, offset + 2), 16) / 255)
        gl!.uniform3f(gl!.getUniformLocation(program, `u_color${uniform}`), rgb[0]!, rgb[1]!, rgb[2]!)
      }
    }
    updateColors()
    resize()
    return { draw, resize, updateColors, destroy }
  }
  catch {
    destroy()
    return undefined
  }
}

function mountCloudscape(host: HTMLElement) {
  const canvas = host.querySelector('canvas')!
  const lifecycle = new AbortController()
  const { signal } = lifecycle
  const reducedMotion = matchMedia('(prefers-reduced-motion: reduce)')
  let renderer: Renderer | undefined
  let frame = 0
  let visible = false
  let elapsed = 0
  let lastTime = 0
  let lastDraw = 0

  function stop() {
    cancelAnimationFrame(frame)
    frame = 0
    lastTime = 0
    lastDraw = 0
  }
  function animate(now: number) {
    frame = 0
    if (lastTime)
      elapsed += (now - lastTime) / 1000
    lastTime = now
    if (now - lastDraw >= 1000 / 30) {
      renderer?.draw(elapsed)
      lastDraw = now
    }
    frame = requestAnimationFrame(animate)
  }
  function update() {
    stop()
    renderer?.draw(elapsed)
    if (renderer && visible && !document.hidden && !reducedMotion.matches)
      frame = requestAnimationFrame(animate)
  }
  function initialize() {
    stop()
    renderer?.destroy()
    renderer = createRenderer(host, canvas)
    update()
  }
  // Theme changes only update uniforms; they keep the context and animation clock.
  function updateColors() {
    renderer?.updateColors()
    update()
  }
  const resizeObserver = new ResizeObserver(() => {
    renderer?.resize()
    update()
  })
  const intersectionObserver = new IntersectionObserver(([entry]) => {
    visible = entry?.isIntersecting ?? false
    update()
  })
  const themeObserver = new MutationObserver(updateColors)
  resizeObserver.observe(host)
  intersectionObserver.observe(host)
  themeObserver.observe(document.documentElement, {
    attributes: true,
    attributeFilter: ['data-theme'],
  })
  reducedMotion.addEventListener('change', update, { signal })
  matchMedia('(prefers-color-scheme: dark)').addEventListener('change', updateColors, { signal })
  document.addEventListener('visibilitychange', update, { signal })
  window.addEventListener('resize', () => {
    renderer?.resize()
    update()
  }, { signal })
  canvas.addEventListener('webglcontextlost', (event) => {
    event.preventDefault()
    stop()
    renderer?.destroy()
    renderer = undefined
  }, { signal })
  canvas.addEventListener('webglcontextrestored', initialize, { signal })
  initialize()

  return () => {
    stop()
    lifecycle.abort()
    resizeObserver.disconnect()
    intersectionObserver.disconnect()
    themeObserver.disconnect()
    renderer?.destroy()
  }
}

let cleanups: (() => void)[] = []
function cleanup() {
  cleanups.forEach(dispose => dispose())
  cleanups = []
}
function setup() {
  cleanup()
  cleanups = [
    ...document.querySelectorAll<HTMLElement>('[data-cloudscape]'),
  ].map(mountCloudscape)
}
function restore(event: PageTransitionEvent) {
  if (event.persisted)
    setup()
}

setup()
document.addEventListener('astro:page-load', setup)
document.addEventListener('astro:before-swap', cleanup)
window.addEventListener('pagehide', cleanup)
window.addEventListener('pageshow', restore)
if (import.meta.hot) {
  import.meta.hot.dispose(() => {
    cleanup()
    document.removeEventListener('astro:page-load', setup)
    document.removeEventListener('astro:before-swap', cleanup)
    window.removeEventListener('pagehide', cleanup)
    window.removeEventListener('pageshow', restore)
  })
}
