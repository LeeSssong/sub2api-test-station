(() => {
  const ns = 'http://www.w3.org/2000/svg';
  const stage = document.querySelector('.constellation');
  const svg = document.querySelector('.connections');
  const defs = svg.querySelector('defs');
  const routeLayer = svg.querySelector('.route-layer');
  const particleLayer = svg.querySelector('.particle-layer');
  const mark = document.querySelector('.mark-wrap');
  const buttons = [...document.querySelectorAll('.provider')];
  const reduced = matchMedia('(prefers-reduced-motion: reduce)');
  const charges = [...document.querySelectorAll('.arm-charge')];
  const armLayer = document.querySelector('.arm-travelers');
  const aura = document.querySelector('.core-aura');
  const flash = document.querySelector('.core-flash');
  const timing = { cycle: 7.2, charge: .35, collect: .8, core: 1.15, peak: 1.48, launch: 1.62 };
  const clamp = n => Math.max(0, Math.min(1, n));
  const smooth = n => { const x = clamp(n); return x * x * (3 - 2 * x); };
  let selected = '', hovered = '', focused = '';
  let inView = false, paused = true, quiet = reduced.matches;
  let motionFrame = 0, drawFrame = 0, lastTick = null;
  let elapsed = 0, feedback = null, routes = [], speed = 1, tailSize = 1;

  function node(tag, attributes = {}) {
    const el = document.createElementNS(ns, tag);
    Object.entries(attributes).forEach(([key, value]) => el.setAttribute(key, value));
    return el;
  }
  const arms = [[65, 82], [293, 82], [65, 276], [293, 276]].map(([x, y]) => {
    const path = node('path', { d: `M ${x} ${y} L 179 179`, fill: 'none', stroke: '#d8ffff', 'stroke-width': 5, 'stroke-linecap': 'round' });
    const bloom = path.cloneNode();
    bloom.setAttribute('stroke-width', '10');
    bloom.setAttribute('filter', 'url(#arm-bloom)');
    const light = node('g', { class: 'arm-traveler' });
    light.append(bloom, path);
    armLayer.append(light);
    return { light, path, bloom, length: path.getTotalLength() };
  });

  function segment(path, length, head, size) {
    path.setAttribute('stroke-dasharray', `${size} ${length + size}`);
    path.setAttribute('stroke-dashoffset', String(size - head));
  }
  function pointAt(route, distance) {
    const u = clamp(distance / route.length) * (route.samples.length - 1);
    const i = Math.min(route.samples.length - 2, Math.floor(u));
    const a = route.samples[i], b = route.samples[i + 1], f = u - i;
    return { x: a.x + (b.x - a.x) * f, y: a.y + (b.y - a.y) * f };
  }
  function hideSignal(route) {
    route.head.style.opacity = '0';
    route.tail.style.opacity = '0';
    route.bloom.style.opacity = '0';
    route.particles.dataset.distance = '-1';
  }
  function paintSignal(route, age, velocity) {
    const distance = age * velocity;
    const arrival = route.length / velocity;
    const tailStart = Math.max(0, distance - tailSize);
    const tailEnd = Math.min(route.length, distance);
    const fadeIn = smooth(age / .1);
    route.particles.dataset.distance = age >= 0 ? distance.toFixed(3) : '-1';
    route.particles.dataset.speed = velocity.toFixed(3);
    if (age < 0 || tailStart >= route.length) {
      hideSignal(route);
    } else {
      const a = pointAt(route, tailStart), b = pointAt(route, tailEnd);
      const points = [];
      const steps = Math.max(2, Math.ceil((tailEnd - tailStart) / 2));
      for (let i = 0; i <= steps; i++) {
        const p = pointAt(route, tailStart + (tailEnd - tailStart) * i / steps);
        points.push(`${i ? 'L' : 'M'}${p.x.toFixed(2)} ${p.y.toFixed(2)}`);
      }
      const d = points.join(' ');
      route.tail.setAttribute('d', d);
      route.bloom.setAttribute('d', d);
      route.gradient.setAttribute('x1', a.x);
      route.gradient.setAttribute('y1', a.y);
      route.gradient.setAttribute('x2', b.x);
      route.gradient.setAttribute('y2', b.y);
      route.stops.forEach((stop, i) => {
        const s = tailStart + (tailEnd - tailStart) * i / (route.stops.length - 1);
        const intensity = clamp((s - (distance - tailSize)) / tailSize);
        stop.setAttribute('stop-opacity', (intensity * intensity).toFixed(3));
      });
      route.tail.style.opacity = String(fadeIn * .62);
      route.bloom.style.opacity = String(fadeIn * .12);
      route.head.setAttribute('transform', `translate(${b.x} ${b.y})`);
      route.head.style.opacity = distance <= route.length ? String(fadeIn * .76 * (1 - smooth((age - arrival + .035) / .07))) : '0';
    }
    const sinceArrival = age - arrival;
    return sinceArrival >= 0 && sinceArrival < .55
      ? smooth(sinceArrival / .07) * (1 - smooth((sinceArrival - .07) / .48)) * .24 : 0;
  }

  function paint() {
    const t = elapsed % timing.cycle;
    const manual = feedback !== null;
    const mt = manual ? feedback.time : 0;
    const longest = Math.max(0, ...routes.map(route => route.length / speed));
    let phase = quiet ? 'still' : manual ? 'feedback'
      : t < timing.charge ? 'idle' : t < timing.collect ? 'charging'
      : t < 1.4 ? 'collecting' : t < timing.launch ? 'ignition'
      : t < timing.launch + longest + tailSize / speed ? 'dispatching'
      : t < timing.launch + longest + .55 ? 'settling' : 'rest';
    stage.dataset.phase = phase;
    stage.dataset.motionMode = quiet ? 'still' : manual ? 'feedback' : 'cycle';
    stage.dataset.cycleTime = t.toFixed(3);
    const charge = !quiet && !manual ? smooth((t - timing.charge) / .35) * (1 - smooth((t - timing.collect) / .38)) : 0;
    const inward = clamp((t - timing.collect) / .6);
    arms.forEach(({ light, path, bloom, length }, i) => {
      charges[i].style.opacity = (charge * .22).toFixed(3);
      segment(path, length, inward * (length + 18), 18);
      segment(bloom, length, inward * (length + 24), 24);
      light.style.opacity = !quiet && !manual && t >= timing.collect && t <= 1.4 ? String(Math.sin(Math.PI * inward) * .42) : '0';
    });
    const core = quiet ? 0 : manual
      ? smooth(mt / .1) * (1 - smooth((mt - .1) / .35)) * .55
      : smooth((t - timing.core) / (timing.peak - timing.core)) * (1 - smooth((t - timing.peak) / .48));
    aura.style.opacity = (core * .38).toFixed(3);
    flash.style.opacity = (core * .7).toFixed(3);
    mark.style.setProperty('--energy', core.toFixed(3));
    routes.forEach((route, i) => {
      let reception = 0;
      if (quiet || (manual && route.name !== feedback.name)) hideSignal(route);
      else reception = paintSignal(route, manual ? mt - .16 : t - timing.launch, manual ? speed * 1.5 : speed);
      route.endpoint.setAttribute('r', String(1.1 + reception * .25));
      route.halo.style.opacity = String(.12 + reception * .24);
      buttons[i].style.setProperty('--arrival', reception.toFixed(3));
    });
  }

  // A single RAF owns the automatic loop and the replaceable, one-shot selection feedback.
  function tick(now) {
    motionFrame = 0;
    if (paused) return;
    const dt = lastTick === null ? 0 : Math.min((now - lastTick) / 1000, .05);
    lastTick = now;
    if (feedback) {
      feedback.time += dt;
      const route = routes.find(route => route.name === feedback.name);
      if (!route || feedback.time > .16 + route.length / (speed * 1.5) + .6) {
        feedback = null;
        // Resume into the quiet part of the normal cycle, without replaying a flash.
        elapsed = timing.cycle - 1.4;
      }
    } else elapsed += dt;
    paint();
    motionFrame = requestAnimationFrame(tick);
  }
  function syncMotion() {
    const nextQuiet = reduced.matches;
    if (nextQuiet !== quiet) {
      quiet = nextQuiet;
      feedback = null;
      elapsed = 0;
    }
    const nextPaused = document.hidden || !inView || quiet;
    if (nextPaused !== paused) lastTick = null;
    paused = nextPaused;
    if (paused) {
      cancelAnimationFrame(motionFrame);
      motionFrame = 0;
    } else if (!motionFrame) motionFrame = requestAnimationFrame(tick);
    document.body.dataset.paused = String(paused);
    document.body.dataset.reduced = String(quiet);
    paint();
  }

  function syncFocus() {
    const focus = focused || hovered || selected;
    if (focus) stage.dataset.focus = focus;
    else delete stage.dataset.focus;
    if (selected) stage.dataset.selected = selected;
    else delete stage.dataset.selected;
    buttons.forEach(button => {
      const name = button.dataset.provider;
      button.setAttribute('aria-pressed', String(name === selected));
      button.classList.toggle('is-focused', name === focus);
    });
    [...routeLayer.children, ...particleLayer.children].forEach(group => {
      if (!group.dataset.provider) return;
      const name = group.dataset.provider;
      group.classList.toggle('is-muted', !!focus && name !== focus);
      group.classList.toggle('is-active', name === selected);
      group.classList.toggle('is-hovered', name === focus && name !== selected);
    });
  }
  function activate(name) {
    selected = selected === name ? '' : name;
    feedback = selected && !quiet ? { name: selected, time: 0 } : null;
    if (!feedback) elapsed = timing.cycle - 1.4;
    syncFocus();
    syncMotion();
  }

  function drawConnections() {
    const bounds = stage.getBoundingClientRect(), hub = mark.getBoundingClientRect();
    const sourceX = hub.left - bounds.left + hub.width / 2;
    const sourceY = hub.top - bounds.top + hub.height / 2;
    speed = Math.max(90, bounds.width * .3);
    tailSize = Math.max(18, Math.min(48, bounds.width * .045));
    svg.setAttribute('viewBox', `0 0 ${bounds.width} ${bounds.height}`);
    routeLayer.replaceChildren(); particleLayer.replaceChildren();
    defs.querySelectorAll('.flow-gradient,.tail-gradient').forEach(el => el.remove());
    routes = [];
    buttons.forEach((button, index) => {
      const name = button.dataset.provider, rect = button.getBoundingClientRect();
      const x = rect.right - bounds.left + (bounds.width < 600 ? 8 : 13);
      const y = rect.top - bounds.top + rect.height / 2;
      const distance = Math.max(12, sourceX - x);
      const stemX = sourceX - Math.min(22, distance * .07);
      const d = `M ${sourceX} ${sourceY} H ${stemX} C ${sourceX - distance * .28} ${sourceY}, ${x + distance * .78} ${y}, ${x} ${y}`;
      const gradient = node('linearGradient', { id: `flow-${index}`, class: 'flow-gradient', x1: x, y1: y, x2: sourceX, y2: sourceY, gradientUnits: 'userSpaceOnUse' });
      [[0, ['#5382a5','#588cb0','#6195b7','#568ba8','#5887ac'][index]], [.48, ['#5c9cc9','#6596c2','#60a4ce','#639cc4','#6398c3'][index]], [.8, '#78d0e4'], [.96, '#b6faff'], [1, '#d4ffff']].forEach(([offset, color]) => gradient.append(node('stop', { offset, 'stop-color': color })));
      const tailGradient = node('linearGradient', { id: `tail-${index}`, class: 'tail-gradient', gradientUnits: 'userSpaceOnUse' });
      const stops = Array.from({ length: 6 }, (_, i) => node('stop', { offset: i / 5, 'stop-color': '#bbebe9', 'stop-opacity': 0 }));
      tailGradient.append(...stops); defs.append(gradient, tailGradient);
      const group = node('g', { class: 'route-group', 'data-provider': name });
      group.append(node('path', { d, class: 'route-haze', stroke: `url(#flow-${index})` }));
      const path = node('path', { id: `route-${index}`, d, class: 'route', stroke: `url(#flow-${index})` });
      const halo = node('circle', { cx: x, cy: y, r: 3.5, fill: '#8bd0dc', opacity: .12, filter: 'url(#point-light)' });
      const endpoint = node('circle', { cx: x, cy: y, r: 1.1, fill: '#abdfe5' });
      group.append(path, halo, endpoint); routeLayer.append(group);
      const particles = node('g', { class: 'particle-group', 'data-provider': name });
      const bloom = node('path', { class: 'signal-bloom', stroke: `url(#tail-${index})` });
      const tail = node('path', { class: 'signal-tail', stroke: `url(#tail-${index})` });
      const head = node('g', { class: 'signal-head' });
      head.append(node('circle', { r: 2.1, fill: '#b1eff4', opacity: .28, filter: 'url(#point-light)' }));
      head.append(node('circle', { r: .95, fill: '#e2ffff' }));
      particles.append(bloom, tail, head); particleLayer.append(particles);
      const length = path.getTotalLength(), count = Math.max(2, Math.ceil(length / 2));
      const samples = Array.from({ length: count + 1 }, (_, i) => path.getPointAtLength(length * i / count));
      particles.dataset.length = length.toFixed(3);
      routes.push({ name, path, length, samples, gradient: tailGradient, stops, head, tail, bloom, halo, endpoint, particles });
    });
    syncFocus(); syncMotion();
  }
  function scheduleDraw() {
    cancelAnimationFrame(drawFrame);
    drawFrame = requestAnimationFrame(() => { drawFrame = 0; drawConnections(); });
  }
  buttons.forEach(button => {
    const name = button.dataset.provider;
    button.addEventListener('pointerenter', () => { hovered = name; syncFocus(); });
    button.addEventListener('pointerleave', () => { hovered = ''; syncFocus(); });
    button.addEventListener('focus', () => { focused = button.matches(':focus-visible') ? name : ''; syncFocus(); });
    button.addEventListener('blur', () => { focused = ''; syncFocus(); });
    button.addEventListener('click', () => activate(name));
  });
  document.addEventListener('keydown', event => {
    if (event.key === 'Escape') {
      selected = ''; hovered = ''; focused = ''; feedback = null;
      elapsed = timing.cycle - 1.4;
      syncFocus(); syncMotion();
    }
  });
  reduced.addEventListener('change', syncMotion);
  // Embedded Chromium can update matches without delivering change on an existing query.
  // This low-frequency check also resumes correctly when the preference is turned off.
  setInterval(() => { if (!document.hidden && reduced.matches !== quiet) syncMotion(); }, 250);
  document.addEventListener('visibilitychange', syncMotion);
  new IntersectionObserver(entries => { inView = entries[0].isIntersecting; syncMotion(); }, { threshold: .1 }).observe(stage);
  new ResizeObserver(scheduleDraw).observe(stage);
  document.fonts.ready.then(scheduleDraw);
  scheduleDraw();
})();
