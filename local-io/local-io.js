(function () {
  "use strict";

  var API_HOST = "127.0.0.1";
  var API_PORT = 9080;

  var cardElements = {};
  var tcpConnected = false;
  var requestPending = false;

  function createCardElement(card) {
    var last = card.last || {};
    var sn = (last.serialNumber && last.serialNumber) ? last.serialNumber : "";
    var div = document.createElement("div");
    div.className = "local-io-card";
    div.setAttribute("data-card-id", card.id);
    div.setAttribute("data-sn", sn);

    var html = '<div class="local-io-card-header">';
    html += '<div class="local-io-card-header-inner">';
    html += '<div class="local-io-card-title">Card ' + card.id + '</div>';
    html += '<div class="local-io-card-model">ID: ' + card.id + ' &bull; ' + (card.module || "") + '</div>';
    html += '<div class="local-io-card-sn">SN: ' + (sn || "—") + '</div>';
    html += '</div>';
    html += '<button type="button" class="local-io-btn-reboot" data-reboot-card="' + card.id + '" title="Reboot Card" aria-label="Reboot Card">↻</button>';
    html += '</div>';
    html += '<div class="local-io-card-body">';

    if (last.di && last.di.length) {
      html += '<div class="local-io-section-title">Digital Inputs</div><div class="local-io-do-list">';
      for (var d = 0; d < last.di.length; d++) {
        var diOn = last.di[d];
        html += '<div class="local-io-do-item' + (diOn ? ' local-io-do-item-on' : ' local-io-do-item-off') + '" data-di-index="' + d + '">';
        html += '<div class="local-io-do-info"><div class="local-io-led' + (diOn ? ' active' : '') + '"></div><span class="local-io-do-name">DI-' + (d + 1) + '</span></div>';
        html += '<div class="local-io-val-display local-io-di-val-display"><span class="local-io-di-state' + (diOn ? ' local-io-di-state-on' : ' local-io-di-state-off') + '" data-di="' + d + '">' + (diOn ? "ON" : "OFF") + "</span></div>";
        html += "</div>";
      }
      html += "</div>";
    }
    if (last.do && last.do.length) {
      html += '<div class="local-io-section-title">Digital Outputs</div><div class="local-io-do-list">';
      for (var o = 0; o < last.do.length; o++) {
        var onState = last.do[o];
        html += '<div class="local-io-do-item' + (onState ? ' local-io-do-item-on' : ' local-io-do-item-off') + '" data-do-index="' + o + '">';
        html += '<div class="local-io-do-info"><div class="local-io-led' + (onState ? ' active' : '') + '"></div><span class="local-io-do-name">DO-' + (o + 1) + '</span></div>';
        html += '<div class="local-io-btn-group">';
        html += '<button type="button" class="local-io-btn-ctrl' + (onState ? ' local-io-active-on' : '') + '" data-do-card="' + card.id + '" data-do-index="' + o + '" data-do-state="true">ON</button>';
        html += '<button type="button" class="local-io-btn-ctrl' + (!onState ? ' local-io-active-off' : '') + '" data-do-card="' + card.id + '" data-do-index="' + o + '" data-do-state="false">OFF</button>';
        html += "</div></div>";
      }
      html += "</div>";
    }
    if (last.ai && last.ai.length) {
      html += '<div class="local-io-section-title">Analog Inputs</div><div class="local-io-ai-list">';
      for (var a = 0; a < last.ai.length; a++) {
        var val = last.ai[a];
        var numVal = Number(val);
        var isZero = val == null || val === '' || numVal === 0 || (typeof numVal === 'number' && isNaN(numVal));
        var current = numVal / 1000;
        var pct = isZero ? 0 : Math.max(0, Math.min(100, (current / 20) * 100));
        if (pct > 0 && pct < 2) pct = 2;
        var barZero = isZero || pct <= 0;
        var pctRounded = Math.round(pct / 5) * 5;
        if (pctRounded > 100) pctRounded = 100;
        html += '<div class="local-io-ai-item' + (barZero ? ' local-io-ai-item-bar-zero' : '') + '" data-ai-index="' + a + '">';
        html += '<div class="local-io-ai-info"><span class="local-io-ai-name">AI-' + (a + 1) + '</span><span class="local-io-raw-badge">' + (isZero ? '0.00' : current.toFixed(2)) + 'mA</span></div>';
        html += '<div class="local-io-val-display"><span class="local-io-val-main" data-ai="' + a + '">' + (isZero ? '0' : Math.round(val)) + '</span></div>';
        html += '<div class="local-io-bar-bg local-io-ai-bar' + (barZero ? ' local-io-ai-bar-zero' : '') + '"><div class="local-io-bar-fill local-io-ai-bar-fill local-io-ai-bar-fill-pct-' + pctRounded + '"></div></div>';
        html += '</div>';
      }
      html += "</div>";
    }
    if (last.ao && last.ao.length) {
      html += '<div class="local-io-section-title">Analog Outputs</div><div class="local-io-do-list">';
      for (var b = 0; b < last.ao.length; b++) {
        var raw = Math.round(last.ao[b]);
        var aoType = (last.aoType && last.aoType[b]) ? last.aoType[b] : "4-20mA";
        var normalized;
        var unit;
        if (aoType === "0-10V") {
          normalized = (raw / 10000) * 10;
          unit = "V";
        } else {
          normalized = ((raw - 4000) / 16000) * 16 + 4;
          unit = "mA";
        }
        html += '<div class="local-io-do-item" data-ao-index="' + b + '" data-ao-type="' + aoType + '">';
        html += '<div class="local-io-do-info"><span class="local-io-do-name local-io-do-name-fixed">AO-' + (b + 1) + '</span><span class="local-io-raw-badge">' + normalized.toFixed(2) + unit + '</span></div>';
        html += '<div class="local-io-btn-group local-io-btn-group-ao">';
        html += '<span class="local-io-val-main local-io-val-main-inline" data-ao="' + b + '">' + raw + '</span>';
        html += '<button type="button" class="local-io-btn-ctrl local-io-btn-ctrl-set" data-ao-card="' + card.id + '" data-ao-index="' + b + '">SET</button></div>';
        html += '</div>';
      }
      html += "</div>";
    }
    if (last.error) {
      html += '<div class="local-io-card-error"><span class="local-io-error">' + last.error + "</span></div>";
    }

    html += "</div>";
    div.innerHTML = html;
    return div;
  }

  function updateCardValues(cardEl, card) {
    var last = card.last || {};
    var v;

    if (last.di) {
      for (var d = 0; d < last.di.length; d++) {
        var diOn = last.di[d];
        var diItem = cardEl.querySelector('.local-io-do-item[data-di-index="' + d + '"]');
        if (diItem) {
          diItem.className = "local-io-do-item" + (diOn ? " local-io-do-item-on" : " local-io-do-item-off");
          var led = diItem.querySelector('.local-io-led');
          if (led) led.className = "local-io-led" + (diOn ? " active" : "");
          v = diItem.querySelector('[data-di="' + d + '"]');
          if (v) {
            v.textContent = diOn ? "ON" : "OFF";
            v.className = "local-io-di-state" + (diOn ? " local-io-di-state-on" : " local-io-di-state-off");
          }
        }
      }
    }
    if (last.do) {
      for (var o = 0; o < last.do.length; o++) {
        var onState = last.do[o];
        var doItem = cardEl.querySelector('.local-io-do-item[data-do-index="' + o + '"]');
        if (!doItem) continue;
        doItem.className = "local-io-do-item" + (onState ? " local-io-do-item-on" : " local-io-do-item-off");
        var led = doItem.querySelector('.local-io-led');
        if (led) led.className = "local-io-led" + (onState ? " active" : "");
        var onBtn = doItem.querySelector('[data-do-state="true"]');
        var offBtn = doItem.querySelector('[data-do-state="false"]');
        if (onBtn) onBtn.className = "local-io-btn-ctrl" + (onState ? " local-io-active-on" : "");
        if (offBtn) offBtn.className = "local-io-btn-ctrl" + (!onState ? " local-io-active-off" : "");
      }
    }
    if (last.ai) {
      for (var a = 0; a < last.ai.length; a++) {
        var val = last.ai[a];
        var numVal = Number(val);
        var isZero = val == null || val === '' || numVal === 0 || (typeof numVal === 'number' && isNaN(numVal));
        v = cardEl.querySelector('.local-io-ai-item[data-ai-index="' + a + '"] .local-io-val-main');
        if (v) v.textContent = isZero ? '0' : Math.round(val);
        var aiItem = cardEl.querySelector('.local-io-ai-item[data-ai-index="' + a + '"]');
        if (aiItem) {
          var rawBadge = aiItem.querySelector('.local-io-raw-badge');
          if (rawBadge) rawBadge.textContent = (isZero ? '0.00' : (numVal / 1000).toFixed(2)) + 'mA';
          var current = numVal / 1000;
          var pct = isZero ? 0 : Math.max(0, Math.min(100, (current / 20) * 100));
          if (pct > 0 && pct < 2) pct = 2;
          var pctRounded = Math.round(pct / 5) * 5;
          if (pctRounded > 100) pctRounded = 100;
          var fill = aiItem.querySelector('.local-io-ai-bar-fill');
          if (fill) fill.className = 'local-io-bar-fill local-io-ai-bar-fill local-io-ai-bar-fill-pct-' + pctRounded;
          var bar = aiItem.querySelector('.local-io-ai-bar');
          if (bar) bar.className = 'local-io-bar-bg local-io-ai-bar' + (isZero || pct <= 0 ? ' local-io-ai-bar-zero' : '');
          var barZero = isZero || pct <= 0;
          if (barZero) aiItem.classList.add('local-io-ai-item-bar-zero');
          else aiItem.classList.remove('local-io-ai-item-bar-zero');
        }
      }
    }
    if (last.ao) {
      for (var b = 0; b < last.ao.length; b++) {
        var raw = Math.round(last.ao[b]);
        var aoItem = cardEl.querySelector('.local-io-do-item[data-ao-index="' + b + '"]');
        if (aoItem) {
          v = aoItem.querySelector('.local-io-val-main-inline');
          if (v) v.textContent = raw;
          var aoType = (last.aoType && last.aoType[b]) ? last.aoType[b] : "4-20mA";
          aoItem.setAttribute("data-ao-type", aoType);
          var normalized;
          var unit;
          if (aoType === "0-10V") {
            normalized = (raw / 10000) * 10;
            unit = "V";
          } else {
            normalized = ((raw - 4000) / 16000) * 16 + 4;
            unit = "mA";
          }
          var badge = aoItem.querySelector('.local-io-raw-badge');
          if (badge) badge.textContent = normalized.toFixed(2) + unit;
        }
      }
    }
  }

  function renderCards(data) {
    var container = document.getElementById("app-container");
    var statusEl = document.getElementById("status");
    if (!container) return;

    var raw = typeof data === "string" ? data : (data && data.body != null ? data.body : null);
    if (!raw) {
      try {
        raw = JSON.stringify(data);
      } catch (e) {}
    }
    if (typeof raw === "string") {
      try {
        data = JSON.parse(raw);
      } catch (e) {
        hideLoading();
        if (statusEl) statusEl.textContent = "Monitor and control local IO cards";
        container.innerHTML = '<p class="local-io-error">Invalid JSON</p>';
        cardElements = {};
        return;
      }
    }

    var cards = data.cards || [];
    tcpConnected = data.tcpConnected || false;

    if (statusEl) statusEl.textContent = "Monitor and control local IO cards";
    hideLoading();

    if (cards.length === 0) {
      hideLoading();
      if (statusEl) statusEl.textContent = "Monitor and control local IO cards";
      container.innerHTML = '<p class="local-io-hint">No cards. Ensure cm-utils is running and has detected IO cards.</p>';
      cardElements = {};
      return;
    }

    var currentIds = {};
    for (var c = 0; c < cards.length; c++) currentIds[String(cards[c].id)] = true;

    var banner = container.querySelector(".local-io-banner");
    if (tcpConnected && !banner) {
      banner = document.createElement("span");
      banner.className = "local-io-banner";
      banner.textContent = "Control disabled (TCP connected)";
      container.insertBefore(banner, container.firstChild);
    } else if (!tcpConnected && banner) {
      banner.parentNode.removeChild(banner);
    }

    var cardsContainer = container.querySelector(".local-io-cards-inner");
    if (!cardsContainer) {
      var hint = container.querySelector(".local-io-hint");
      if (hint) hint.parentNode.removeChild(hint);
      if (tcpConnected) {
        banner = document.createElement("span");
        banner.className = "local-io-banner";
        banner.textContent = "Control disabled (TCP connected)";
        container.appendChild(banner);
      }
      cardsContainer = document.createElement("div");
      cardsContainer.className = "local-io-cards-inner";
      container.appendChild(cardsContainer);
    }

    for (var id in cardElements) {
      if (!currentIds[id]) {
        var oldEl = cardElements[id].el;
        if (oldEl.parentNode === cardsContainer) cardsContainer.removeChild(oldEl);
        delete cardElements[id];
      }
    }

    for (var i = 0; i < cards.length; i++) {
      var card = cards[i];
      var cardId = String(card.id);
      var sn = (card.last && card.last.serialNumber) ? String(card.last.serialNumber) : "";
      var entry = cardElements[cardId];
      var ref = cardsContainer.children[i];

      if (entry && entry.sn === sn) {
        updateCardValues(entry.el, card);
      } else {
        if (entry) {
          if (entry.el.parentNode === cardsContainer) cardsContainer.removeChild(entry.el);
          delete cardElements[cardId];
        }
        var el = createCardElement(card);
        cardElements[cardId] = { el: el, sn: sn };
        cardsContainer.insertBefore(el, ref);
      }
    }

    var doBtns = container.querySelectorAll(".local-io-btn-ctrl[data-do-card]");
    for (var b = 0; b < doBtns.length; b++) doBtns[b].disabled = tcpConnected;
    var aoSetBtns = container.querySelectorAll(".local-io-btn-ctrl-set");
    for (var s = 0; s < aoSetBtns.length; s++) aoSetBtns[s].disabled = tcpConnected;
    var rebootBtns = container.querySelectorAll(".local-io-btn-reboot");
    for (var r = 0; r < rebootBtns.length; r++) rebootBtns[r].disabled = tcpConnected;
  }

  function writeDo(cardId, index, state) {
    return cockpit
      .http({ address: API_HOST, port: API_PORT })
      .post(
        "/api/local-io/" + encodeURIComponent(cardId) + "/write-do",
        JSON.stringify({ index: parseInt(index, 10), state: state === true || state === "true" }),
        { "Content-Type": "application/json" }
      );
  }

  var aoModalCardId = null;
  var aoModalChannel = null;
  var aoModalType = "4-20mA";
  var aoValueMode = "normalized";

  function getRawRange(aoType) {
    if (aoType === "0-10V") return { min: 0, max: 10000 };
    return { min: 4000, max: 20000 };
  }

  function rawToNormalized(raw, aoType) {
    if (aoType === "0-10V") return (raw / 10000) * 10;
    return ((raw - 4000) / 16000) * 16 + 4;
  }

  function normalizedToRaw(normalized, aoType) {
    if (aoType === "0-10V") return Math.round((normalized / 10) * 10000);
    return Math.round(((normalized - 4) / 16) * 16000 + 4000);
  }

  function updateAOValueDisplay() {
    var slider = document.getElementById("local-io-ao-modal-slider");
    var input = document.getElementById("local-io-ao-modal-value");
    var label = document.getElementById("local-io-ao-modal-value-label");
    if (!slider || !input || !label) return;
    var rawRange = getRawRange(aoModalType);
    if (aoValueMode === "normalized") {
      var minNorm = aoModalType === "0-10V" ? 0 : 4;
      var maxNorm = aoModalType === "0-10V" ? 10 : 20;
      var unit = aoModalType === "0-10V" ? "V" : "mA";
      label.textContent = "Normalized Value (" + minNorm + "-" + maxNorm + unit + ")";
      slider.min = minNorm;
      slider.max = maxNorm;
      slider.step = "0.01";
      input.min = minNorm;
      input.max = maxNorm;
      input.step = "0.01";
      var v = parseFloat(slider.value) || minNorm;
      v = Math.max(minNorm, Math.min(maxNorm, v));
      slider.value = v.toFixed(2);
      input.value = v.toFixed(2);
    } else {
      label.textContent = "Raw Value (" + rawRange.min + "-" + rawRange.max + ")";
      slider.min = rawRange.min;
      slider.max = rawRange.max;
      slider.step = "1";
      input.min = rawRange.min;
      input.max = rawRange.max;
      input.step = "1";
      var v = parseInt(slider.value, 10) || rawRange.min;
      v = Math.max(rawRange.min, Math.min(rawRange.max, v));
      slider.value = v;
      input.value = v;
    }
  }

  function updateAOTypeButtons() {
    var display = document.getElementById("local-io-ao-modal-current-type");
    var btn420 = document.getElementById("local-io-ao-type-4-20");
    var btn010 = document.getElementById("local-io-ao-type-0-10");
    if (display) display.textContent = aoModalType;
    if (btn420) btn420.className = "local-io-btn-ctrl-modal" + (aoModalType === "4-20mA" ? " local-io-active-on" : "");
    if (btn010) btn010.className = "local-io-btn-ctrl-modal" + (aoModalType === "0-10V" ? " local-io-active-on" : "");
    updateAOValueDisplay();
  }

  function showAOModal(cardId, channel, currentRaw, aoType) {
    aoModalCardId = cardId;
    aoModalChannel = channel;
    aoModalType = aoType || "4-20mA";
    aoValueMode = "normalized";
    var toggle = document.getElementById("local-io-ao-value-mode-toggle");
    if (toggle) toggle.checked = true;
    var title = document.getElementById("local-io-ao-modal-title");
    var channelEl = document.getElementById("local-io-ao-modal-channel");
    var slider = document.getElementById("local-io-ao-modal-slider");
    var input = document.getElementById("local-io-ao-modal-value");
    if (title) title.textContent = "Card " + cardId + " - AO-" + (channel + 1);
    if (channelEl) channelEl.textContent = "Current: " + currentRaw + " | Target: " + currentRaw;
    var rawRange = getRawRange(aoModalType);
    var clampedRaw = Math.max(rawRange.min, Math.min(rawRange.max, currentRaw));
    var normalized = rawToNormalized(clampedRaw, aoModalType);
    updateAOTypeButtons();
    if (slider) slider.value = aoValueMode === "normalized" ? normalized.toFixed(2) : String(clampedRaw);
    if (input) input.value = aoValueMode === "normalized" ? normalized.toFixed(2) : String(clampedRaw);
    updateAOValueDisplay();
    var modal = document.getElementById("local-io-ao-modal");
    if (modal) modal.classList.remove("local-io-modal-hidden");
    function syncFromSlider() {
      if (aoValueMode === "normalized") {
        input.value = parseFloat(slider.value).toFixed(2);
      } else {
        input.value = slider.value;
      }
      if (channelEl) channelEl.textContent = "Current: " + currentRaw + " | Target: " + (aoValueMode === "normalized" ? normalizedToRaw(parseFloat(input.value), aoModalType) : input.value);
    }
    function syncFromInput() {
      var val = aoValueMode === "normalized" ? parseFloat(input.value) : parseInt(input.value, 10);
      var rawRange = getRawRange(aoModalType);
      if (isNaN(val)) val = aoValueMode === "normalized" ? (aoModalType === "0-10V" ? 0 : 4) : rawRange.min;
      if (aoValueMode === "normalized") {
        var minN = aoModalType === "0-10V" ? 0 : 4;
        var maxN = aoModalType === "0-10V" ? 10 : 20;
        val = Math.max(minN, Math.min(maxN, val));
        slider.value = val.toFixed(2);
        input.value = val.toFixed(2);
      } else {
        val = Math.max(rawRange.min, Math.min(rawRange.max, val));
        slider.value = val;
        input.value = val;
      }
      if (channelEl) channelEl.textContent = "Current: " + currentRaw + " | Target: " + (aoValueMode === "normalized" ? normalizedToRaw(parseFloat(input.value), aoModalType) : input.value);
    }
    if (slider) slider.oninput = syncFromSlider;
    if (input) input.oninput = syncFromInput;
  }

  function hideAOModal() {
    aoModalCardId = null;
    aoModalChannel = null;
    var modal = document.getElementById("local-io-ao-modal");
    if (modal) modal.classList.add("local-io-modal-hidden");
  }

  function writeAo(cardId, index, value) {
    return cockpit
      .http({ address: API_HOST, port: API_PORT })
      .post(
        "/api/local-io/" + encodeURIComponent(cardId) + "/write-ao",
        JSON.stringify({ index: parseInt(index, 10), value: parseInt(value, 10) }),
        { "Content-Type": "application/json" }
      );
  }

  function rebootCard(cardId) {
    return cockpit
      .http({ address: API_HOST, port: API_PORT })
      .post("/api/local-io/" + encodeURIComponent(cardId) + "/reboot", "{}", { "Content-Type": "application/json" });
  }

  function writeAOType(cardId, index, mode) {
    return cockpit
      .http({ address: API_HOST, port: API_PORT })
      .post(
        "/api/local-io/" + encodeURIComponent(cardId) + "/write-aotype",
        JSON.stringify({ index: parseInt(index, 10), mode: mode }),
        { "Content-Type": "application/json" }
      );
  }

  function onContainerClick(e) {
    var btn = e.target;
    if (btn.classList && btn.classList.contains("local-io-btn-ctrl") && btn.getAttribute("data-do-card")) {
      if (btn.disabled) return;
      var cardId = btn.getAttribute("data-do-card");
      var index = btn.getAttribute("data-do-index");
      var state = btn.getAttribute("data-do-state") === "true";
      if (!cardId || index === null) return;
      writeDo(cardId, index, state).then(fetchLocalIO, function (err) {
        var container = document.getElementById("app-container");
        if (container) container.innerHTML = '<p class="local-io-error">Error: ' + (err.message || err) + "</p>";
      });
      return;
    }
    if (btn.classList && btn.classList.contains("local-io-btn-ctrl-set")) {
      if (btn.disabled) return;
      var cardId = btn.getAttribute("data-ao-card");
      var index = btn.getAttribute("data-ao-index");
      if (!cardId || index === null) return;
      var aoItem = btn.closest(".local-io-do-item");
      var rawSpan = aoItem ? aoItem.querySelector(".local-io-val-main-inline") : null;
      var currentRaw = rawSpan ? parseInt(rawSpan.textContent, 10) : 4000;
      if (isNaN(currentRaw)) currentRaw = 4000;
      var aoType = (aoItem && aoItem.getAttribute("data-ao-type")) || "4-20mA";
      showAOModal(cardId, parseInt(index, 10), currentRaw, aoType);
      return;
    }
    if (btn.classList && btn.classList.contains("local-io-btn-reboot")) {
      if (btn.disabled) return;
      var cardId = btn.getAttribute("data-reboot-card");
      if (!cardId) return;
      if (!window.confirm("Are you sure you want to reboot Card " + cardId + "?")) return;
      rebootCard(cardId)
        .then(fetchLocalIO)
        .catch(function (err) {
          var container = document.getElementById("app-container");
          if (container) container.innerHTML = '<p class="local-io-error">Error: ' + (err.message || err) + "</p>";
        });
      return;
    }
  }

  function showLoading() {
    var el = document.getElementById("local-io-loading");
    if (el) el.classList.remove("local-io-loading-hidden");
  }

  function hideLoading() {
    var el = document.getElementById("local-io-loading");
    if (el) el.classList.add("local-io-loading-hidden");
  }

  function fetchLocalIO() {
    if (requestPending) return;
    requestPending = true;

    var container = document.getElementById("app-container");
    var statusEl = document.getElementById("status");
    var hadCards = Object.keys(cardElements).length > 0;
    if (!hadCards) showLoading();
    if (statusEl && !hadCards) statusEl.textContent = "";

    cockpit
      .http({ address: API_HOST, port: API_PORT })
      .get("/api/local-io")
      .then(renderCards)
      .catch(function (err) {
        hideLoading();
        if (statusEl) statusEl.textContent = "Monitor and control local IO cards";
        if (container) {
          container.innerHTML = '<p class="local-io-error">Error: ' + (err.message || err) + "</p>";
        }
        cardElements = {};
      })
      .finally(function () {
        requestPending = false;
      });
  }

  function init() {
    var appContainer = document.getElementById("app-container");
    if (appContainer) {
      appContainer.addEventListener("click", onContainerClick);
    }
    var aoModalCancel = document.getElementById("local-io-ao-modal-cancel");
    var aoModalSet = document.getElementById("local-io-ao-modal-set");
    var aoModalClose = document.getElementById("local-io-ao-modal-close");
    var aoModalBackdrop = document.querySelector("#local-io-ao-modal .local-io-modal-backdrop");
    if (aoModalCancel) aoModalCancel.addEventListener("click", hideAOModal);
    if (aoModalClose) aoModalClose.addEventListener("click", hideAOModal);
    if (aoModalBackdrop) aoModalBackdrop.addEventListener("click", hideAOModal);
    var aoType420 = document.getElementById("local-io-ao-type-4-20");
    var aoType010 = document.getElementById("local-io-ao-type-0-10");
    if (aoType420) {
      aoType420.addEventListener("click", function () {
        if (aoModalCardId == null || aoModalChannel == null) return;
        if (!window.confirm("Change AO-" + (aoModalChannel + 1) + " type to 4-20mA? Reboot card may be required.")) return;
        aoModalType = "4-20mA";
        updateAOTypeButtons();
        writeAOType(aoModalCardId, aoModalChannel, "4-20mA").then(fetchLocalIO).catch(function (err) {
          var container = document.getElementById("app-container");
          if (container) container.innerHTML = '<p class="local-io-error">Error: ' + (err.message || err) + "</p>";
        });
      });
    }
    if (aoType010) {
      aoType010.addEventListener("click", function () {
        if (aoModalCardId == null || aoModalChannel == null) return;
        if (!window.confirm("Change AO-" + (aoModalChannel + 1) + " type to 0-10V? Reboot card may be required.")) return;
        aoModalType = "0-10V";
        updateAOTypeButtons();
        writeAOType(aoModalCardId, aoModalChannel, "0-10V").then(fetchLocalIO).catch(function (err) {
          var container = document.getElementById("app-container");
          if (container) container.innerHTML = '<p class="local-io-error">Error: ' + (err.message || err) + "</p>";
        });
      });
    }
    var aoValueToggle = document.getElementById("local-io-ao-value-mode-toggle");
    if (aoValueToggle) {
      aoValueToggle.addEventListener("change", function () {
        aoValueMode = this.checked ? "normalized" : "raw";
        updateAOValueDisplay();
      });
    }
    if (aoModalSet) {
      aoModalSet.addEventListener("click", function () {
        if (aoModalCardId == null || aoModalChannel == null) return;
        var input = document.getElementById("local-io-ao-modal-value");
        var raw;
        if (aoValueMode === "normalized") {
          var normalized = input ? parseFloat(input.value) : 4;
          if (isNaN(normalized)) normalized = 4;
          raw = normalizedToRaw(normalized, aoModalType);
        } else {
          raw = input ? parseInt(input.value, 10) : 4000;
          if (isNaN(raw)) raw = 4000;
        }
        var rawRange = getRawRange(aoModalType);
        raw = Math.max(rawRange.min, Math.min(rawRange.max, raw));
        writeAo(aoModalCardId, aoModalChannel, raw)
          .then(function () {
            hideAOModal();
            fetchLocalIO();
          })
          .catch(function (err) {
            hideAOModal();
            var container = document.getElementById("app-container");
            if (container) container.innerHTML = '<p class="local-io-error">Error: ' + (err.message || err) + "</p>";
          });
      });
    }
    fetchLocalIO();
    setInterval(fetchLocalIO, 500);
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
