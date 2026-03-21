// dashboard.js — SDD Dashboard WebSocket + ECharts client
// Vanilla JS, no framework. Three responsibilities:
// 1. WS connection manager with auto-reconnect
// 2. DOM updaters for KPI, pipelines, errors
// 3. ECharts managers for 5 chart types
"use strict";

// ---------------------------------------------------------------------------
// 1. WebSocket Connection Manager
// ---------------------------------------------------------------------------

var ws = null;
var reconnectDelay = 1000;
var reconnectTimer = null;

function setIndicator(color) {
  var dot = document.querySelector(".header .dot");
  if (!dot) return;
  var colors = { green: "#00e676", yellow: "#ffc107", red: "#ff5252" };
  dot.style.background = colors[color] || colors.red;
}

function connect() {
  setIndicator("yellow");
  ws = new WebSocket("ws://" + location.host + "/ws");

  ws.onopen = function () {
    reconnectDelay = 1000;
    setIndicator("green");
  };

  ws.onmessage = function (evt) {
    var msg;
    try { msg = JSON.parse(evt.data); } catch (_) { return; }
    dispatch(msg.type, msg.data);
  };

  ws.onclose = function () {
    setIndicator("red");
    scheduleReconnect();
  };

  ws.onerror = function () {
    ws.close();
  };
}

function scheduleReconnect() {
  if (reconnectTimer) return;
  setIndicator("yellow");
  reconnectTimer = setTimeout(function () {
    reconnectTimer = null;
    connect();
  }, reconnectDelay);
  reconnectDelay = Math.min(reconnectDelay * 2, 10000);
}

function dispatch(type, data) {
  switch (type) {
    case "kpi":        handleKPI(data);        break;
    case "pipelines":  handlePipelines(data);  break;
    case "errors":     handleErrors(data);     break;
    case "chart:tokens":    updateTokenChart(data);    break;
    case "chart:durations": updateDurationChart(data); break;
    case "chart:cache":     updateCacheChart(data);    break;
    case "chart:verify":    updateVerifyChart(data);   break;
    case "chart:heatmap":   updateHeatmapChart(data);  break;
  }
}

// ---------------------------------------------------------------------------
// 2. DOM Updaters
// ---------------------------------------------------------------------------

function handleKPI(d) {
  setText("kpi-active", d.ActiveChanges);
  setText("kpi-tokens", formatNumber(d.TotalTokens));
  setText("kpi-cache", Math.round(d.CacheHitPct) + "%");
  setText("kpi-errors", d.ErrorCount);
}

function handlePipelines(rows) {
  var tbody = document.getElementById("pipeline-body");
  if (!tbody) return;
  if (!rows || rows.length === 0) {
    tbody.innerHTML = '<tr><td colspan="5" class="empty-state">No active changes</td></tr>';
    return;
  }
  var html = "";
  for (var i = 0; i < rows.length; i++) {
    var r = rows[i];
    html += "<tr>"
      + "<td>" + esc(r.Name) + "</td>"
      + "<td>" + esc(r.CurrentPhase) + "</td>"
      + '<td><div class="progress-bar"><div class="progress-fill" style="width:' + r.ProgressPct + '%"></div></div>'
      + '<span style="font-size:11px;color:#888">' + r.Completed + "/" + r.Total + "</span></td>"
      + "<td>" + formatNumber(r.Tokens) + "</td>"
      + '<td><span class="status-dot ' + esc(r.Status) + '"></span></td>'
      + "</tr>";
  }
  tbody.innerHTML = html;
}

function handleErrors(rows) {
  var tbody = document.getElementById("error-body");
  if (!tbody) return;
  if (!rows || rows.length === 0) {
    tbody.innerHTML = '<tr><td colspan="6" class="empty-state">No errors recorded</td></tr>';
    return;
  }
  var html = "";
  for (var i = 0; i < rows.length; i++) {
    var r = rows[i];
    html += "<tr>"
      + "<td>" + esc(r.Timestamp) + "</td>"
      + "<td>" + esc(r.CommandName) + "</td>"
      + "<td>" + r.ExitCode + "</td>"
      + "<td>" + esc(r.Change) + "</td>"
      + '<td class="fp">' + esc(r.Fingerprint) + "</td>"
      + "<td>" + esc(r.FirstLine) + "</td>"
      + "</tr>";
  }
  tbody.innerHTML = html;
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function setText(id, val) {
  var el = document.getElementById(id);
  if (el) el.textContent = val == null ? "—" : val;
}

function esc(s) {
  if (!s) return "";
  var d = document.createElement("div");
  d.textContent = s;
  return d.innerHTML;
}

function formatNumber(n) {
  if (n == null) return "0";
  return Number(n).toLocaleString();
}

// ---------------------------------------------------------------------------
// 3. ECharts Managers
// ---------------------------------------------------------------------------

var PHASES = ["explore", "propose", "spec", "design", "tasks", "apply", "review", "verify", "clean", "archive"];

// Chart instances — lazy initialized
var charts = {
  tokens: null,
  durations: null,
  cache: null,
  verify: null,
  heatmap: null,
};

// Accumulated data for incremental charts (capped to prevent unbounded growth)
var MAX_CHART_POINTS = 2000;
var tokenSeriesData = {};  // change -> [{ts, tokens}]
var cacheSeriesData = { hit: [], miss: [] }; // [{ts, count}] grouped
var verifySeriesData = []; // [{ts, cmd, passed}]

function capArray(arr) {
  if (arr.length > MAX_CHART_POINTS) arr.splice(0, arr.length - MAX_CHART_POINTS);
}

function getChart(name) {
  if (charts[name]) return charts[name];
  var el = document.getElementById("chart-" + name);
  if (!el) return null;
  var instance = echarts.init(el, null, { renderer: "canvas" });
  charts[name] = instance;
  return instance;
}

function baseOption() {
  return {
    backgroundColor: "transparent",
    textStyle: { color: "#e0e0e0" },
    grid: { left: 60, right: 30, top: 40, bottom: 40 },
    tooltip: { trigger: "axis", backgroundColor: "#16213e", borderColor: "#333", textStyle: { color: "#e0e0e0" } },
  };
}

// --- Token Chart (line) ---

function updateTokenChart(rows) {
  if (!rows || rows.length === 0) return;

  for (var i = 0; i < rows.length; i++) {
    var r = rows[i];
    var key = r.Change || "unknown";
    if (!tokenSeriesData[key]) tokenSeriesData[key] = [];
    tokenSeriesData[key].push([r.Timestamp, r.Tokens]);
    capArray(tokenSeriesData[key]);
  }

  var chart = getChart("tokens");
  if (!chart) return;

  var changeNames = Object.keys(tokenSeriesData);
  var series = [];
  for (var c = 0; c < changeNames.length; c++) {
    series.push({
      name: changeNames[c],
      type: "line",
      smooth: true,
      symbol: "circle",
      symbolSize: 4,
      data: tokenSeriesData[changeNames[c]],
    });
  }

  var opt = baseOption();
  opt.legend = { data: changeNames, textStyle: { color: "#e0e0e0" }, top: 5 };
  opt.xAxis = { type: "time", axisLine: { lineStyle: { color: "#333" } }, splitLine: { show: false } };
  opt.yAxis = { type: "value", name: "Tokens", axisLine: { lineStyle: { color: "#333" } }, splitLine: { lineStyle: { color: "#222" } } };
  opt.series = series;
  chart.setOption(opt);
}

// --- Duration Chart (horizontal bar) ---

function updateDurationChart(rows) {
  if (!rows || rows.length === 0) return;

  var chart = getChart("durations");
  if (!chart) return;

  var phases = [];
  var values = [];
  for (var i = 0; i < rows.length; i++) {
    phases.push(rows[i].Phase);
    values.push(rows[i].AvgDurationMs);
  }

  var opt = baseOption();
  opt.xAxis = { type: "value", name: "ms", axisLine: { lineStyle: { color: "#333" } }, splitLine: { lineStyle: { color: "#222" } } };
  opt.yAxis = { type: "category", data: phases, axisLine: { lineStyle: { color: "#333" } }, inverse: true };
  opt.series = [{
    type: "bar",
    data: values,
    itemStyle: { color: "#bb86fc", borderRadius: [0, 3, 3, 0] },
    barMaxWidth: 20,
  }];
  opt.tooltip.trigger = "item";
  chart.setOption(opt, true);
}

// --- Cache Chart (stacked area) ---

function updateCacheChart(rows) {
  if (!rows || rows.length === 0) return;

  // Group incoming rows by timestamp -> hit/miss counts
  var buckets = {};
  for (var i = 0; i < rows.length; i++) {
    var ts = rows[i].Timestamp;
    if (!buckets[ts]) buckets[ts] = { hit: 0, miss: 0 };
    if (rows[i].Cached) {
      buckets[ts].hit++;
    } else {
      buckets[ts].miss++;
    }
  }

  var timestamps = Object.keys(buckets).sort();
  for (var t = 0; t < timestamps.length; t++) {
    var ts = timestamps[t];
    cacheSeriesData.hit.push([ts, buckets[ts].hit]);
    cacheSeriesData.miss.push([ts, buckets[ts].miss]);
  }
  capArray(cacheSeriesData.hit);
  capArray(cacheSeriesData.miss);

  var chart = getChart("cache");
  if (!chart) return;

  var opt = baseOption();
  opt.legend = { data: ["Hit", "Miss"], textStyle: { color: "#e0e0e0" }, top: 5 };
  opt.xAxis = { type: "time", axisLine: { lineStyle: { color: "#333" } }, splitLine: { show: false } };
  opt.yAxis = { type: "value", name: "Count", axisLine: { lineStyle: { color: "#333" } }, splitLine: { lineStyle: { color: "#222" } } };
  opt.series = [
    { name: "Hit",  type: "line", stack: "cache", areaStyle: { opacity: 0.5 }, data: cacheSeriesData.hit,  itemStyle: { color: "#00e676" }, lineStyle: { color: "#00e676" } },
    { name: "Miss", type: "line", stack: "cache", areaStyle: { opacity: 0.5 }, data: cacheSeriesData.miss, itemStyle: { color: "#ff5252" }, lineStyle: { color: "#ff5252" } },
  ];
  chart.setOption(opt);
}

// --- Verify Chart (scatter) ---

function updateVerifyChart(rows) {
  if (!rows || rows.length === 0) return;

  for (var i = 0; i < rows.length; i++) {
    verifySeriesData.push(rows[i]);
  }
  capArray(verifySeriesData);

  var chart = getChart("verify");
  if (!chart) return;

  // Collect unique command names for y-axis categories
  var cmdSet = {};
  for (var i = 0; i < verifySeriesData.length; i++) {
    cmdSet[verifySeriesData[i].CommandName] = true;
  }
  var commands = Object.keys(cmdSet);

  var passData = [];
  var failData = [];
  for (var i = 0; i < verifySeriesData.length; i++) {
    var r = verifySeriesData[i];
    var point = [r.Timestamp, r.CommandName];
    if (r.Passed) {
      passData.push(point);
    } else {
      failData.push(point);
    }
  }

  var opt = baseOption();
  opt.legend = { data: ["Passed", "Failed"], textStyle: { color: "#e0e0e0" }, top: 5 };
  opt.xAxis = { type: "time", axisLine: { lineStyle: { color: "#333" } }, splitLine: { show: false } };
  opt.yAxis = { type: "category", data: commands, axisLine: { lineStyle: { color: "#333" } } };
  opt.series = [
    { name: "Passed", type: "scatter", data: passData, symbolSize: 10, itemStyle: { color: "#00e676" } },
    { name: "Failed", type: "scatter", data: failData, symbolSize: 10, itemStyle: { color: "#ff5252" } },
  ];
  opt.tooltip.trigger = "item";
  chart.setOption(opt, true);
}

// --- Heatmap Chart ---

function updateHeatmapChart(rows) {
  if (!rows || rows.length === 0) return;

  var chart = getChart("heatmap");
  if (!chart) return;

  var statusColor = {
    pending:     "#333",
    in_progress: "#ffc107",
    completed:   "#00e676",
    skipped:     "#555",
  };

  // Collect unique changes preserving order
  var changeSet = {};
  var changes = [];
  for (var i = 0; i < rows.length; i++) {
    if (!changeSet[rows[i].change]) {
      changeSet[rows[i].change] = true;
      changes.push(rows[i].change);
    }
  }

  // Build heatmap data: [phaseIndex, changeIndex, statusValue]
  // We encode status as a number for the visualMap, then use piecewise coloring.
  var statusToVal = { pending: 0, in_progress: 1, completed: 2, skipped: 3 };
  var data = [];
  for (var i = 0; i < rows.length; i++) {
    var r = rows[i];
    var px = PHASES.indexOf(r.phase);
    var cy = changes.indexOf(r.change);
    if (px < 0 || cy < 0) continue;
    data.push([px, cy, statusToVal[r.status] != null ? statusToVal[r.status] : 0]);
  }

  var opt = {
    backgroundColor: "transparent",
    textStyle: { color: "#e0e0e0" },
    tooltip: {
      trigger: "item",
      backgroundColor: "#16213e",
      borderColor: "#333",
      textStyle: { color: "#e0e0e0" },
      formatter: function (p) {
        var statuses = ["pending", "in_progress", "completed", "skipped"];
        return esc(PHASES[p.data[0]]) + " / " + esc(changes[p.data[1]]) + "<br>" + statuses[p.data[2]];
      },
    },
    grid: { left: 120, right: 30, top: 20, bottom: 50 },
    xAxis: { type: "category", data: PHASES, axisLine: { lineStyle: { color: "#333" } }, splitArea: { show: true, areaStyle: { color: ["rgba(255,255,255,0.02)", "rgba(255,255,255,0.04)"] } } },
    yAxis: { type: "category", data: changes, axisLine: { lineStyle: { color: "#333" } } },
    visualMap: {
      show: false,
      min: 0,
      max: 3,
      inRange: {
        color: [statusColor.pending, statusColor.in_progress, statusColor.completed, statusColor.skipped],
      },
    },
    series: [{
      type: "heatmap",
      data: data,
      label: { show: false },
      itemStyle: { borderColor: "#1a1a2e", borderWidth: 2, borderRadius: 2 },
    }],
  };
  chart.setOption(opt, true);
}

// ---------------------------------------------------------------------------
// 4. Tab Logic
// ---------------------------------------------------------------------------

var TAB_NAMES = ["tokens", "durations", "cache", "verify", "heatmap"];

function initTabs() {
  var btns = document.querySelectorAll(".tab-btn");
  for (var i = 0; i < btns.length; i++) {
    btns[i].addEventListener("click", onTabClick);
  }
  // Activate default tab
  selectTab("tokens");
}

function onTabClick(e) {
  var tab = e.currentTarget.getAttribute("data-tab");
  if (tab) selectTab(tab);
}

function selectTab(name) {
  // Update button active states
  var btns = document.querySelectorAll(".tab-btn");
  for (var i = 0; i < btns.length; i++) {
    var btn = btns[i];
    if (btn.getAttribute("data-tab") === name) {
      btn.classList.add("active");
    } else {
      btn.classList.remove("active");
    }
  }

  // Show/hide panels
  var panels = document.querySelectorAll(".chart-panel");
  for (var i = 0; i < panels.length; i++) {
    panels[i].style.display = "none";
  }
  var target = document.getElementById("chart-" + name);
  if (target) {
    target.style.display = "block";
    // Resize chart if already initialized
    if (charts[name]) {
      charts[name].resize();
    }
  }
}

// Handle window resize — resize all active chart instances
window.addEventListener("resize", function () {
  for (var key in charts) {
    if (charts[key]) charts[key].resize();
  }
});

// ---------------------------------------------------------------------------
// Boot
// ---------------------------------------------------------------------------

document.addEventListener("DOMContentLoaded", function () {
  initTabs();
  connect();
});
