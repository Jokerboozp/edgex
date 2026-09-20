// Syntax highlighting — only explicit language-tagged code blocks; skip HTML examples and UI regions.
function shouldSkipBlock(block) {
  if (block.closest('.hero-actions, .hero-action-row, .hero-stage-spatial, .hero-section, .hero-copy, .doc-card, .card-grid, .hero-terminal-pill')) {
    return true;
  }

  const pre = block.closest('pre');
  if (!pre) {
    return true;
  }

  const hasLanguage = [...block.classList].some((cls) => cls.startsWith('language-'));
  const inHighlight = Boolean(pre.closest('.highlight, .highlighter-rouge'));

  if (!hasLanguage && !inHighlight) {
    return true;
  }

  const code = block.textContent;
  if (/^\s*</.test(code) || /<\/?[a-z][\s\S]*>/i.test(code)) {
    return true;
  }

  return false;
}

function highlightCode() {
  document.querySelectorAll('pre code').forEach((block) => {
    if (shouldSkipBlock(block)) {
      return;
    }

    let code = block.textContent;

    code = code.replace(/\b(const|let|var|function|if|else|for|while|return|class|import|export)\b/g, '<span class="tok-keyword">$1</span>');
    code = code.replace(/"([^"]*)"/g, '<span class="tok-string">"$1"</span>');
    code = code.replace(/'([^']*)'/g, '<span class="tok-string">\'$1\'</span>');
    code = code.replace(/\b\d+\b/g, '<span class="tok-number">$&</span>');

    block.innerHTML = code;
  });
}

function addCopyButtons() {
  document.querySelectorAll('pre').forEach((pre) => {
    const code = pre.querySelector('code');
    if (!code || shouldSkipBlock(code)) {
      return;
    }

    if (pre.querySelector('.copy-button')) {
      return;
    }

    const button = document.createElement('button');
    button.className = 'copy-button';
    button.textContent = '复制';

    button.addEventListener('click', () => {
      navigator.clipboard.writeText(code.textContent).then(() => {
        button.textContent = '已复制!';
        button.classList.add('copied');
        setTimeout(() => {
          button.textContent = '复制';
          button.classList.remove('copied');
        }, 2000);
      });
    });

    pre.appendChild(button);
  });
}

window.addEventListener('DOMContentLoaded', () => {
  initHeroVisual();
  highlightCode();
  addCopyButtons();
  initTypewriter();
  initThemeToggle();
});

// Hero visual — cycle through six industrial-AI core effects, one minute each
// All effects share a unified fx-viewport (300×300px) to guarantee visual size consistency.
// Maximum outer radius is clamped to 130px so every scene occupies the same visual mass.
function initHeroVisual() {
  var container = document.querySelector('[data-hero-visual]');
  if (!container) return;

  var logoSvg = '<svg class="fx-logo-svg" viewBox="0 0 64 64" fill="none" xmlns="http://www.w3.org/2000/svg">' +
    '<path class="fx-logo-hex" d="M32 4L56 18v28L32 60 8 46V18L32 4z" stroke="currentColor" stroke-width="2.2" stroke-linejoin="round"/>' +
    '<path class="fx-logo-inner" d="M32 14l14 8v16l-14 8-14-8V22l14-8z" stroke="currentColor" stroke-width="1.2" stroke-linejoin="round" opacity="0.6"/>' +
    '<path class="fx-logo-rays" d="M32 4v10M56 18L46 22M56 46L46 42M32 60V50M8 46l10-4M8 18l10 4" stroke="currentColor" stroke-width="1" opacity="0.4"/>' +
    '<g class="fx-logo-nodes"><circle cx="32" cy="4" r="2.2" fill="currentColor"/><circle cx="56" cy="18" r="2.2" fill="currentColor"/><circle cx="56" cy="46" r="2.2" fill="currentColor"/><circle cx="32" cy="60" r="2.2" fill="currentColor"/><circle cx="8" cy="46" r="2.2" fill="currentColor"/><circle cx="8" cy="18" r="2.2" fill="currentColor"/></g>' +
    '<circle class="fx-logo-core" cx="32" cy="32" r="5.5" fill="currentColor"/>' +
    '<circle class="fx-logo-orbit" cx="32" cy="32" r="11" stroke="currentColor" stroke-width="1.2" stroke-dasharray="3 4" opacity="0.75" fill="none"/>' +
    '</svg>';
  var logo = '<div class="fx-logo">' + logoSvg + '</div>';

  function generateNodes(count, radius, size, delayOffset) {
    var html = '';
    for (var i = 0; i < count; i++) {
      var angle = (360 / count * i).toFixed(1);
      var delay = ((i * 0.12) + (delayOffset || 0)).toFixed(2);
      html += '<span class="fx-node" style="--a:' + angle + 'deg;--r:' + radius + 'px;--d:' + size + 'px;--delay:' + delay + 's"></span>';
    }
    return html;
  }

  function generateTicks(count, radius, length) {
    var html = '';
    for (var i = 0; i < count; i++) {
      var angle = (360 / count * i).toFixed(1);
      html += '<span class="fx-tick" style="--a:' + angle + 'deg;--r:' + radius + 'px;--l:' + length + 'px"></span>';
    }
    return html;
  }

  function buildScene(type) {
    var innerContent = '';

    if (type === 'core') {
      innerContent =
        '<div class="fx-core">' + logo +
          '<div class="fx-core__ring fx-core__ring--solid"></div>' +
          '<div class="fx-core__ring fx-core__ring--dashed"></div>' +
          '<div class="fx-core__ring fx-core__ring--dotted"></div>' +
          '<div class="fx-core__ring fx-core__ring--outer-ticks">' + generateTicks(36, 130, 5) + '</div>' +
          '<div class="fx-core__nodes fx-core__nodes--inner">' + generateNodes(12, 105, 5, 0) + '</div>' +
          '<div class="fx-core__nodes fx-core__nodes--outer">' + generateNodes(12, 130, 6, 0.5) + '</div>' +
          '<span class="fx-core__pulse-wave"></span>' +
        '</div>';

    } else if (type === 'lattice') {
      var latticeNodes = [];
      var ringConfigs = [{ r: 65, offset: -90, size: 5 }, { r: 120, offset: -90, size: 6 }];
      for (var ri = 0; ri < ringConfigs.length; ri++) {
        var rc = ringConfigs[ri];
        for (var ni = 0; ni < 4; ni++) {
          var ang = (90 * ni + rc.offset) * Math.PI / 180;
          latticeNodes.push({
            x: Math.round(rc.r * Math.cos(ang)),
            y: Math.round(rc.r * Math.sin(ang)),
            ring: ri + 1, size: rc.size, delay: (ni * 0.2 + ri * 0.3).toFixed(2)
          });
        }
      }
      var latticeLinks = [];
      for (var si = 0; si < 4; si++) {
        latticeLinks.push([-1, si, 'flow']);
        latticeLinks.push([si, si + 4, 'flow']);
      }
      for (var ri2 = 0; ri2 < 2; ri2++) {
        for (var ci = 0; ci < 4; ci++) {
          latticeLinks.push([ri2 * 4 + ci, ri2 * 4 + (ci + 1) % 4, '']);
        }
      }
      var guideSvg = '<circle class="fx-lattice__guide" cx="0" cy="0" r="65"/><circle class="fx-lattice__guide" cx="0" cy="0" r="120"/>';
      var linkSvg = '';
      for (var l = 0; l < latticeLinks.length; l++) {
        var lk = latticeLinks[l];
        var n1 = lk[0] === -1 ? { x: 0, y: 0 } : latticeNodes[lk[0]];
        var n2 = lk[1] === -1 ? { x: 0, y: 0 } : latticeNodes[lk[1]];
        var flowCls = lk[2] === 'flow' ? ' fx-lattice__link--flow' : '';
        linkSvg += '<line class="fx-lattice__link' + flowCls + '" x1="' + n1.x + '" y1="' + n1.y + '" x2="' + n2.x + '" y2="' + n2.y + '"/>';
      }
      var nodeSvg = '';
      for (var n = 0; n < latticeNodes.length; n++) {
        var nd = latticeNodes[n];
        nodeSvg += '<circle class="fx-lattice__node fx-lattice__node--r' + nd.ring + '" cx="' + nd.x + '" cy="' + nd.y + '" r="' + nd.size + '" style="--delay:' + nd.delay + 's"/>';
      }
      var pulseHtml = '';
      for (var sp = 0; sp < 4; sp++) {
        var r1n = latticeNodes[sp];
        var r2n = latticeNodes[sp + 4];
        pulseHtml += '<span class="fx-lattice__pulse" style="offset-path: path(\'M 150 150 L ' + (r1n.x + 150) + ' ' + (r1n.y + 150) + ' L ' + (r2n.x + 150) + ' ' + (r2n.y + 150) + '\');--delay:' + (sp * 0.35).toFixed(2) + 's"></span>';
      }
      innerContent =
        '<div class="fx-lattice">' + logo +
          '<svg class="fx-lattice__svg" viewBox="-150 -150 300 300">' +
            '<g class="fx-lattice__links">' + guideSvg + linkSvg + '</g>' +
            '<g class="fx-lattice__nodes">' + nodeSvg + '</g>' +
          '</svg>' +
          '<div class="fx-lattice__pulses">' + pulseHtml + '</div>' +
          '<div class="fx-lattice__scan"></div>' +
          '<div class="fx-lattice__boundary"></div>' +
        '</div>';

    } else if (type === 'radar') {
      var blips = '';
      var samplePoints = [{r: 45, a: 40}, {r: 80, a: 130}, {r: 105, a: 220}, {r: 60, a: 310}];
      for (var b = 0; b < samplePoints.length; b++) {
        var pt = samplePoints[b];
        blips +=
          '<div class="fx-radar__blip-wrapper" style="--r:' + pt.r + 'px;--a:' + pt.a + 'deg;--delay:' + (b * 0.7).toFixed(2) + 's">' +
            '<span class="fx-radar__blip"></span>' +
            (b % 2 === 0 ? '<span class="fx-radar__target-box"></span>' : '') +
          '</div>';
      }
      innerContent =
        '<div class="fx-radar">' + logo +
          '<div class="fx-radar__outer-ring"></div>' +
          '<div class="fx-radar__rings">' +
            '<span class="fx-radar__ring r1"></span>' +
            '<span class="fx-radar__ring r2"></span>' +
            '<span class="fx-radar__ring r3"></span>' +
          '</div>' +
          '<div class="fx-radar__crosshair"></div>' +
          '<div class="fx-radar__degrees">' + generateTicks(24, 130, 4) + '</div>' +
          '<div class="fx-radar__sweep-arm"><span class="fx-radar__sweep-sector"></span></div>' +
          blips +
        '</div>';

    } else if (type === 'field') {
      var streamAngles = [15, 45, 75, 105, 135, 165, 195, 225, 255, 285, 315, 345];
      var streamLines = '';
      var streamPackets = '';
      for (var s = 0; s < streamAngles.length; s++) {
        var sa = streamAngles[s];
        streamLines += '<div class="fx-field__stream-line" style="--a:' + sa + 'deg"></div>';
        streamPackets += '<div class="fx-field__stream-packet" style="--a:' + sa + 'deg;--delay:' + (s * 0.2).toFixed(2) + 's"></div>';
      }
      innerContent =
        '<div class="fx-field">' + logo +
          '<div class="fx-field__boundary"></div>' +
          '<div class="fx-field__polar-grid"></div>' +
          '<div class="fx-field__cross-axis"></div>' +
          '<div class="fx-field__reticle fx-field__reticle--tl"></div>' +
          '<div class="fx-field__reticle fx-field__reticle--tr"></div>' +
          '<div class="fx-field__reticle fx-field__reticle--bl"></div>' +
          '<div class="fx-field__reticle fx-field__reticle--br"></div>' +
          '<div class="fx-field__ticks">' + generateTicks(24, 130, 4) + '</div>' +
          streamLines +
          streamPackets +
        '</div>';

    } else if (type === 'beacon') {
      var rings = '';
      for (var k = 0; k < 4; k++) {
        rings += '<span class="fx-beacon__ring" style="--delay:' + (k * 0.7).toFixed(2) + 's"></span>';
      }
      innerContent =
        '<div class="fx-beacon">' + logo +
          '<div class="fx-beacon__boundary"></div>' +
          '<div class="fx-beacon__cone"></div>' +
          rings +
        '</div>';

    } else if (type === 'nexus') {
      var neuralClusters = [
        { x: 125, y: 125, delay: '0.2s', size: 3.2 },
        { x: 175, y: 125, delay: '0.8s', size: 3.2 },
        { x: 150, y: 180, delay: '1.4s', size: 3.2 },
        { x: 92,  y: 150, delay: '0.5s', size: 3.5 },
        { x: 125, y: 92,  delay: '1.1s', size: 3.5 },
        { x: 175, y: 92,  delay: '1.7s', size: 3.5 },
        { x: 208, y: 150, delay: '0.9s', size: 3.5 },
        { x: 175, y: 208, delay: '2.1s', size: 3.5 },
        { x: 125, y: 208, delay: '1.3s', size: 3.5 },
        { x: 65,  y: 110, delay: '1.6s', size: 2.8 },
        { x: 150, y: 52,  delay: '0.4s', size: 2.8 },
        { x: 235, y: 110, delay: '2.3s', size: 2.8 },
        { x: 226, y: 182, delay: '1.0s', size: 2.8 },
        { x: 150, y: 248, delay: '1.8s', size: 2.8 },
        { x: 65,  y: 190, delay: '0.7s', size: 2.8 }
      ];
      var synapses = [
        [0, 1, true], [1, 2, true], [2, 0, false],
        [0, 3, true], [0, 4, false], [1, 5, true], [1, 6, true],
        [2, 7, true], [2, 8, false],
        [3, 4, false], [4, 5, true], [5, 6, false],
        [6, 7, true], [7, 8, false], [8, 3, true],
        [3, 9, true], [4, 10, true], [5, 11, false],
        [6, 12, true], [7, 13, true], [8, 14, false],
        [9, 14, false], [10, 11, true], [12, 13, false]
      ];
      var svgContent = '';
      var impulsesSvg = '';
      for (var syn = 0; syn < synapses.length; syn++) {
        var edge = synapses[syn];
        var nA = neuralClusters[edge[0]];
        var nB = neuralClusters[edge[1]];
        var isActive = edge[2];
        var cls = isActive ? 'fx-nexus__synapse fx-nexus__synapse--active' : 'fx-nexus__synapse';
        svgContent += '<line class="' + cls + '" x1="' + nA.x + '" y1="' + nA.y + '" x2="' + nB.x + '" y2="' + nB.y + '"/>';
        if (isActive) {
          var dur = (1.8 + (syn % 4) * 0.4).toFixed(1) + 's';
          var del = ((syn * 0.2) % 2.0).toFixed(2) + 's';
          impulsesSvg += '<circle class="fx-nexus__svg-impulse" r="2.5" style="offset-path: path(\'M ' + nA.x + ' ' + nA.y + ' L ' + nB.x + ' ' + nB.y + '\');--dur:' + dur + ';--delay:' + del + '"/>';
        }
      }
      for (var nn = 0; nn < neuralClusters.length; nn++) {
        var nc = neuralClusters[nn];
        svgContent += '<circle class="fx-nexus__node-halo" cx="' + nc.x + '" cy="' + nc.y + '" style="--delay:' + nc.delay + '"/>';
        svgContent += '<circle class="fx-nexus__node-core" cx="' + nc.x + '" cy="' + nc.y + '" r="' + nc.size + '" style="--delay:' + nc.delay + '"/>';
      }
      innerContent =
        '<div class="fx-nexus">' + logo +
          '<div class="fx-nexus__boundary"></div>' +
          '<div class="fx-nexus__ticks">' + generateTicks(24, 130, 4) + '</div>' +
          '<svg class="fx-nexus__svg" viewBox="0 0 300 300">' +
            '<defs><clipPath id="nexus-clip"><circle cx="150" cy="150" r="128"/></clipPath></defs>' +
            '<g clip-path="url(#nexus-clip)">' + svgContent + impulsesSvg + '</g>' +
          '</svg>' +
        '</div>';

    } else if (type === 'pulse') {
      var pRings = '';
      for (var pi = 0; pi < 3; pi++) {
        pRings += '<span class="fx-pulse__wave" style="--delay:' + (pi * 0.8).toFixed(2) + 's"></span>';
      }
      innerContent =
        '<div class="fx-pulse">' + logo +
          '<div class="fx-pulse__boundary"></div>' +
          '<div class="fx-pulse__ticks">' + generateTicks(36, 125, 6) + '</div>' +
          pRings +
        '</div>';

    } else if (type === 'matrix') {
      var dots = '';
      for (var mi = 0; mi < 36; mi++) {
        dots += '<span class="fx-matrix__dot" style="--delay:' + (Math.random() * 2).toFixed(2) + 's"></span>';
      }
      innerContent =
        '<div class="fx-matrix">' + logo +
          '<div class="fx-matrix__frame">' +
            '<div class="fx-matrix__grid">' + dots + '</div>' +
            '<div class="fx-matrix__scanbar"></div>' +
          '</div>' +
        '</div>';

    } else if (type === 'orbit') {
      var oRings = [
        { w: 250, h: 250, rot: 15,  dur: '8s',  dir: 'normal' },
        { w: 220, h: 220, rot: 75,  dur: '12s', dir: 'reverse' },
        { w: 260, h: 260, rot: 135, dur: '10s', dir: 'normal' }
      ];
      var ringHtml = '';
      for (var r = 0; r < oRings.length; r++) {
        var item = oRings[r];
        ringHtml +=
          '<div class="fx-orbit__ring-3d" style="--w:' + item.w + 'px;--h:' + item.h + 'px;--rot:' + item.rot + 'deg;--dur:' + item.dur + ';--dir:' + item.dir + '">' +
            '<span class="fx-orbit__satellite"></span>' +
          '</div>';
      }
      innerContent =
        '<div class="fx-orbit">' + logo +
          '<div class="fx-orbit__boundary"></div>' +
          '<div class="fx-orbit__ticks">' + generateTicks(24, 130, 4) + '</div>' +
          ringHtml +
        '</div>';

    } else if (type === 'beam') {
      var beamAxes = '';
      var beamPulses = '';
      var receivers = '';
      var angles = [0, 45, 90, 135, 180, 225, 270, 315];
      for (var a = 0; a < angles.length; a++) {
        var bang = angles[a];
        if (bang < 180) {
          beamAxes += '<div class="fx-beam__axis" style="--a:' + bang + 'deg"></div>';
        }
        beamPulses += '<div class="fx-beam__pulse" style="--a:' + bang + 'deg;--delay:' + (a * 0.25).toFixed(2) + 's"></div>';
        receivers += '<div class="fx-beam__receiver" style="--a:' + bang + 'deg"></div>';
      }
      innerContent =
        '<div class="fx-beam">' + logo +
          '<div class="fx-beam__boundary"></div>' +
          beamAxes + beamPulses + receivers +
        '</div>';

    } else if (type === 'flux') {
      var fluxSparks = '';
      for (var fs = 0; fs < 8; fs++) {
        var fAngle = (fs * 45) * Math.PI / 180;
        var fxX = Math.round(150 + 105 * Math.cos(fAngle));
        var fxY = Math.round(150 + 105 * Math.sin(fAngle));
        fluxSparks += '<circle class="fx-flux__spark" cx="' + fxX + '" cy="' + fxY + '" r="2.8" style="--delay:' + (fs * 0.25).toFixed(2) + 's"/>';
      }
      innerContent =
        '<div class="fx-flux">' + logo +
          '<div class="fx-flux__boundary"></div>' +
          '<div class="fx-flux__ticks">' + generateTicks(24, 130, 4) + '</div>' +
          '<svg class="fx-flux__svg" viewBox="0 0 300 300">' +
            '<defs><clipPath id="flux-clip"><circle cx="150" cy="150" r="128"/></clipPath></defs>' +
            '<g clip-path="url(#flux-clip)">' +
              '<circle class="fx-flux__arc-cw" cx="150" cy="150" r="105"/>' +
              '<circle class="fx-flux__arc-ccw" cx="150" cy="150" r="80"/>' +
              '<circle class="fx-flux__arc-cw" cx="150" cy="150" r="58" style="animation-duration: 5s; stroke-width: 1;"/>' +
              fluxSparks +
            '</g>' +
          '</svg>' +
        '</div>';

    } else if (type === 'swarm') {
      var pathGuard = 'M 150 105 A 45 45 0 1 1 150 195 A 45 45 0 1 1 150 105 Z';
      var pathPatrolA = 'M 150 45 C 240 45, 250 255, 150 255 C 50 255, 60 45, 150 45 Z';
      var pathPatrolB = 'M 45 150 C 45 60, 255 50, 255 150 C 255 240, 45 230, 45 150 Z';
      var pathForager = 'M 150 150 C 260 30, 260 270, 150 150 C 40 270, 40 30, 150 150 Z';
      var beesHtml = '';
      for (var g = 0; g < 8; g++) {
        var gDur = (4.0 + (g % 3) * 0.5).toFixed(1) + 's';
        var gDel = (g * 0.45).toFixed(2) + 's';
        var jx = ((Math.random() - 0.5) * 4).toFixed(1) + 'px';
        var jy = ((Math.random() - 0.5) * 4).toFixed(1) + 'px';
        beesHtml += '<circle class="fx-swarm__bee--guard" r="2.2" style="offset-path: path(\'' + pathGuard + '\');--dur:' + gDur + ';--delay:' + gDel + ';--jx:' + jx + ';--jy:' + jy + ';"/>';
      }
      for (var p = 0; p < 20; p++) {
        var pPath = (p % 2 === 0) ? pathPatrolA : pathPatrolB;
        var pDur = (7.5 + (p % 4) * 0.8).toFixed(1) + 's';
        var pDel = (p * 0.35).toFixed(2) + 's';
        var pJx = ((Math.random() - 0.5) * 6).toFixed(1) + 'px';
        var pJy = ((Math.random() - 0.5) * 6).toFixed(1) + 'px';
        beesHtml += '<circle class="fx-swarm__bee--patrol" r="2.6" style="offset-path: path(\'' + pPath + '\');--dur:' + pDur + ';--delay:' + pDel + ';--jx:' + pJx + ';--jy:' + pJy + ';"/>';
      }
      for (var f = 0; f < 8; f++) {
        var fDur = (9.0 + (f % 2) * 1.2).toFixed(1) + 's';
        var fDel = (f * 0.8).toFixed(2) + 's';
        var cx = (Math.sin(f * 1.2) * 18).toFixed(1) + 'px';
        var cy = (Math.cos(f * 1.2) * 18).toFixed(1) + 'px';
        var fJx = ((Math.random() - 0.5) * 8).toFixed(1) + 'px';
        var fJy = ((Math.random() - 0.5) * 8).toFixed(1) + 'px';
        beesHtml += '<circle class="fx-swarm__bee--forager" r="3.0" style="offset-path: path(\'' + pathForager + '\');--dur:' + fDur + ';--delay:' + fDel + ';--cx:' + cx + ';--cy:' + cy + ';--jx:' + fJx + ';--jy:' + fJy + ';"/>';
      }
      innerContent =
        '<div class="fx-swarm">' + logo +
          '<div class="fx-swarm__hive-core"></div>' +
          '<div class="fx-swarm__boundary"></div>' +
          '<div class="fx-swarm__ticks">' + generateTicks(24, 130, 4) + '</div>' +
          '<svg class="fx-swarm__svg" viewBox="0 0 300 300">' +
            '<defs><clipPath id="swarm-clip"><circle cx="150" cy="150" r="128"/></clipPath></defs>' +
            '<g clip-path="url(#swarm-clip)">' +
              '<path class="fx-swarm__orbit-guide" d="' + pathGuard + '"/>' +
              '<path class="fx-swarm__orbit-guide" d="' + pathPatrolA + '"/>' +
              '<path class="fx-swarm__orbit-guide" d="' + pathPatrolB + '"/>' +
              '<path class="fx-swarm__orbit-guide" d="' + pathForager + '"/>' +
              beesHtml +
            '</g>' +
          '</svg>' +
        '</div>';
    }

    return '<div class="fx-scene"><div class="fx-viewport">' + innerContent + '</div></div>';
  }

  // 六套场景（原十二套）：取原清单首尾各三套 ——
  // 前三套 core/lattice/radar 偏「架构」，后三套 beam/flux/swarm 偏「流动」，
  // 两半各自成组，60s 一轮交替，视觉密度不重复也不偏科。
  var effects = ['core', 'lattice', 'radar', 'beam', 'flux', 'swarm'];
  container.innerHTML = effects.map(buildScene).join('');

  var scenes = container.querySelectorAll('.fx-scene');
  if (!scenes.length) return;

  scenes[0].classList.add('fx-scene--active');

  if (window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
    return;
  }

  var current = 0;
  var SCENE_DURATION = 60000;
  var sceneTimer = null;

  // 统一切换函数 / Unified scene switcher
  function switchScene(next) {
    scenes[current].classList.remove('fx-scene--active');
    current = next % scenes.length;
    scenes[current].classList.add('fx-scene--active');
  }

  function startAutoCycle() {
    if (sceneTimer) clearInterval(sceneTimer);
    sceneTimer = setInterval(function () {
      switchScene(current + 1);
    }, SCENE_DURATION);
  }

  startAutoCycle();

  // 双击手动切换下一个 / Double-click to advance to next scene
  container.addEventListener('dblclick', function () {
    switchScene(current + 1);
    startAutoCycle(); // 重置自动轮播计时器 / reset auto-cycle timer
  });
}

// Theme toggle — dark/light switch with localStorage
function initThemeToggle() {
  var storageKey = 'edgeCore-docs-theme';
  var root = document.documentElement;
  var button = document.querySelector('[data-theme-toggle]');

  function syncTheme(theme) {
    root.setAttribute('data-theme', theme);
    // 亮/暗图标由 CSS 依据 data-theme 切换，此处只维护无障碍状态
    if (button) button.setAttribute('aria-pressed', String(theme === 'dark'));
  }

  var current = root.getAttribute('data-theme') || 'light';
  syncTheme(current);

  if (button) {
    button.addEventListener('click', function () {
      var now = root.getAttribute('data-theme') === 'light' ? 'light' : 'dark';
      var next = now === 'dark' ? 'light' : 'dark';
      syncTheme(next);
      localStorage.setItem(storageKey, next);
    });
  }
}

// Typewriter effect — cycles through edgeCore key features
function initTypewriter() {
  var tw = document.querySelector('.hero-typewriter .typewriter-text');
  var cursor = document.querySelector('.hero-typewriter .typewriter-cursor');
  if (!tw) return;

  var lines = [
    // 「13 种工业协议统一接入」已固定写在首屏副标题（.hero-subhead-mono）上，
    // 此处不再重复，避免副标题与打字机同时出现同一句话
    'ShadowCore 内存影子真源',
    'ScanEngine 10ms 级调度内核',
    '工业级 SLA · lag P95 <100ms',
    'AI 辅助设备接入与协议解析',
    '二进制 · 零依赖 · 跨平台 rpm/deb',
    'MCP 服务发现与注册,多版本兼容',
    '北向接口 · OpenAPI 3.0',
    '无缝对接 SCADA 系统,支持数据采集与处理',
  ];
  var lineIndex = 0, i = 0, deleting = false;

  function tick() {
    var text = lines[lineIndex];
    if (!deleting) {
      i++;
      tw.textContent = text.slice(0, i);
      if (i >= text.length) { deleting = true; setTimeout(tick, 3000); return; }
    } else {
      i--;
      tw.textContent = text.slice(0, i);
      if (i <= 0) { deleting = false; lineIndex = (lineIndex + 1) % lines.length; setTimeout(tick, 500); return; }
    }
    setTimeout(tick, deleting ? 50 : 120);
  }
  tick();
}
