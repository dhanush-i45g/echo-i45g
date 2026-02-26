"use strict";

// ============================================================================
// Echo i45G — Glassmorphism Frontend
// ============================================================================

const API = "/api";
let computers = [];
let deleteTargetId = null;
let csvFile = null;

// ============================================================================
// API Layer
// ============================================================================

async function apiCall(url, opts = {}) {
  const res = await fetch(url, opts);
  const json = await res.json();
  if (!json.success) throw new Error(json.error);
  return json.data;
}

const api = {
  list:      ()         => apiCall(`${API}/computers`),
  add:       (d)        => apiCall(`${API}/computers`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(d) }),
  update:    (id, d)    => apiCall(`${API}/computers/${id}`, { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify(d) }),
  remove:    (id)       => apiCall(`${API}/computers/${id}`, { method: "DELETE" }),
  ping:      (id)       => apiCall(`${API}/ping/${id}`),
  pingAll:   ()         => apiCall(`${API}/ping-all`),
  uploadCSV: (fd)       => apiCall(`${API}/computers/upload`, { method: "POST", body: fd }),
};

// ============================================================================
// Parallax Background — moves on mouse hover
// ============================================================================

function initParallax() {
  const bg = document.getElementById("parallax-bg");
  if (!bg) return;

  let targetX = 0, targetY = 0, currentX = 0, currentY = 0;
  const STRENGTH = 18; // px of movement

  document.addEventListener("mousemove", (e) => {
    const cx = window.innerWidth / 2;
    const cy = window.innerHeight / 2;
    targetX = ((e.clientX - cx) / cx) * STRENGTH;
    targetY = ((e.clientY - cy) / cy) * STRENGTH;
  });

  function animate() {
    currentX += (targetX - currentX) * 0.06;
    currentY += (targetY - currentY) * 0.06;
    bg.style.transform = `translate(${-currentX}px, ${-currentY}px)`;
    requestAnimationFrame(animate);
  }
  animate();
}

// ============================================================================
// Floating Particles
// ============================================================================

function initParticles() {
  const canvas = document.getElementById("particles");
  if (!canvas) return;
  const ctx = canvas.getContext("2d");
  let w, h;
  const particles = [];
  const COUNT = 50;

  function resize() {
    w = canvas.width = window.innerWidth;
    h = canvas.height = window.innerHeight;
  }
  resize();
  window.addEventListener("resize", resize);

  for (let i = 0; i < COUNT; i++) {
    particles.push({
      x: Math.random() * w,
      y: Math.random() * h,
      r: Math.random() * 1.8 + 0.4,
      dx: (Math.random() - 0.5) * 0.3,
      dy: (Math.random() - 0.5) * 0.2,
      o: Math.random() * 0.4 + 0.1,
    });
  }

  function draw() {
    ctx.clearRect(0, 0, w, h);
    for (const p of particles) {
      p.x += p.dx;
      p.y += p.dy;
      if (p.x < 0) p.x = w;
      if (p.x > w) p.x = 0;
      if (p.y < 0) p.y = h;
      if (p.y > h) p.y = 0;
      ctx.beginPath();
      ctx.arc(p.x, p.y, p.r, 0, Math.PI * 2);
      ctx.fillStyle = `rgba(124, 143, 255, ${p.o})`;
      ctx.fill();
    }
    requestAnimationFrame(draw);
  }
  draw();
}

// ============================================================================
// Card Rendering
// ============================================================================

let cardIndex = 0;

function renderCard(computer, statusData = null) {
  const el = document.getElementById(`card-${computer.id}`);
  const status = statusData ? statusData.status : null;
  const checkedAt = statusData ? statusData.checkedAt : null;
  const cls = status === "ON" ? "card--on" : status === "OFF" ? "card--off" : "card--idle";
  const badge = status || "UNKNOWN";
  const time = checkedAt ? `Checked: ${checkedAt}` : "Not checked yet";
  const hasKey = computer.sshKey && computer.sshKey.length > 0;

  const html = `
    <div class="card__indicator"><span class="card__dot"></span></div>
    <div class="card__body">
      <h2 class="card__name">${esc(computer.name)}</h2>
      <p class="card__ip">${esc(computer.ip)}${hasKey ? ' <span class="card__key-badge" title="SSH Key">🔑</span>' : ''}</p>
    </div>
    <div class="card__status"><span class="card__badge">${badge}</span></div>
    <div class="card__footer">
      <span class="card__time">${time}</span>
      <div class="card__actions">
        <button class="btn-action btn-action--edit" title="Edit">
          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
            <path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/>
            <path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/>
          </svg>
        </button>
        <button class="btn-action btn-action--delete" title="Delete">
          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
            <polyline points="3 6 5 6 21 6"/>
            <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
          </svg>
        </button>
        <button class="btn-check">
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
            <circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/>
          </svg>
          Check
        </button>
      </div>
    </div>
  `;

  if (el) {
    // Status change flash
    el.classList.add("card--flash");
    setTimeout(() => el.classList.remove("card--flash"), 500);
    el.className = `card ${cls}`;
    el.innerHTML = html;
  } else {
    const card = document.createElement("div");
    card.className = `card ${cls}`;
    card.id = `card-${computer.id}`;
    card.style.setProperty("--delay", `${cardIndex * 0.06}s`);
    card.innerHTML = html;
    document.getElementById("grid").appendChild(card);
    cardIndex++;
  }

  // Event listeners
  const card = document.getElementById(`card-${computer.id}`);
  card.querySelector(".btn-check").addEventListener("click", () => handlePingOne(computer.id));
  card.querySelector(".btn-action--edit").addEventListener("click", () => openEditModal(computer));
  card.querySelector(".btn-action--delete").addEventListener("click", () => openDeleteModal(computer.id, computer.name));
}

function setCardLoading(id, loading) {
  const btn = document.querySelector(`#card-${id} .btn-check`);
  if (!btn) return;
  btn.disabled = loading;
  btn.innerHTML = loading
    ? `<span class="spinner"></span> Checking...`
    : `<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
        <circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/>
      </svg> Check`;
}

function updateSummary(results) {
  const on = results.filter(r => r.status === "ON").length;
  const off = results.filter(r => r.status === "OFF").length;
  animateValue("count-total", computers.length);
  animateValue("count-online", on);
  animateValue("count-offline", off);
}

function renderAllCards() {
  cardIndex = 0;
  document.getElementById("grid").innerHTML = "";
  computers.forEach(c => renderCard(c));
  animateValue("count-total", computers.length);
}

// Animate numbers counting up
function animateValue(id, target) {
  const el = document.getElementById(id);
  const current = parseInt(el.textContent) || 0;
  if (current === target) { el.textContent = target; return; }
  const diff = target - current;
  const steps = Math.min(Math.abs(diff), 15);
  const stepTime = 200 / steps;
  let i = 0;
  const timer = setInterval(() => {
    i++;
    el.textContent = Math.round(current + (diff * i / steps));
    if (i >= steps) { clearInterval(timer); el.textContent = target; }
  }, stepTime);
}

// ============================================================================
// Ping Handlers
// ============================================================================

async function handlePingOne(id) {
  setCardLoading(id, true);
  try {
    const result = await api.ping(id);
    const comp = computers.find(c => c.id === id);
    if (comp) renderCard(comp, result);
  } catch (e) {
    showToast(`Error: ${e.message}`, "error");
  } finally {
    setCardLoading(id, false);
  }
}

async function handlePingAll() {
  const btn = document.getElementById("btn-ping-all");
  btn.disabled = true;
  btn.innerHTML = `<span class="spinner"></span> Checking...`;
  try {
    const results = await api.pingAll();
    results.forEach(r => {
      const comp = computers.find(c => c.id === r.id);
      if (comp) renderCard(comp, r);
    });
    updateSummary(results);
    showToast("All devices checked!", "success");
  } catch (e) {
    showToast(`Error: ${e.message}`, "error");
  } finally {
    btn.disabled = false;
    btn.innerHTML = `<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg> Check All`;
  }
}

// ============================================================================
// CRUD Handlers
// ============================================================================

async function handleFormSubmit(e) {
  e.preventDefault();
  const editId = document.getElementById("form-edit-id").value;
  const name = document.getElementById("form-name").value.trim();
  const ip = document.getElementById("form-ip").value.trim();
  const sshKey = document.getElementById("form-sshkey").value.trim();

  if (!name || !ip) { showToast("Name and IP required", "error"); return; }

  const btn = document.getElementById("form-submit");
  btn.disabled = true;
  btn.textContent = editId ? "Saving..." : "Adding...";

  try {
    if (editId) {
      const updated = await api.update(editId, { name, ip, sshKey });
      const idx = computers.findIndex(c => c.id === parseInt(editId));
      if (idx !== -1) { computers[idx] = updated; renderCard(updated); }
      showToast(`${updated.name} updated!`, "success");
    } else {
      const nc = await api.add({ name, ip, sshKey });
      computers.push(nc);
      renderCard(nc);
      animateValue("count-total", computers.length);
      showToast(`${nc.name} added!`, "success");
    }
    closeModal();
  } catch (e) {
    showToast(`Error: ${e.message}`, "error");
  } finally {
    btn.disabled = false;
    btn.textContent = editId ? "Save Changes" : "Add Device";
  }
}

async function handleDelete() {
  if (!deleteTargetId) return;
  const btn = document.getElementById("delete-confirm");
  btn.disabled = true;
  btn.textContent = "Deleting...";

  try {
    await api.remove(deleteTargetId);
    computers = computers.filter(c => c.id !== deleteTargetId);
    const el = document.getElementById(`card-${deleteTargetId}`);
    if (el) {
      el.style.transition = "all 0.35s ease";
      el.style.opacity = "0";
      el.style.transform = "scale(0.9) translateY(10px)";
      setTimeout(() => el.remove(), 350);
    }
    animateValue("count-total", computers.length);
    showToast("Device deleted", "success");
    closeDeleteModal();
  } catch (e) {
    showToast(`Error: ${e.message}`, "error");
  } finally {
    btn.disabled = false;
    btn.textContent = "Delete";
  }
}

// ============================================================================
// CSV Upload
// ============================================================================

function handleCSVFileSelect(file) {
  if (!file || !file.name.endsWith(".csv")) { showToast("Select a .csv file", "error"); return; }
  csvFile = file;
  document.getElementById("csv-filename").textContent = file.name;
  document.getElementById("csv-submit").disabled = false;

  const reader = new FileReader();
  reader.onload = (e) => {
    const lines = e.target.result.split("\n").filter(l => l.trim());
    const preview = lines.slice(0, 6);
    let html = '<table class="csv-table"><thead><tr><th>Name</th><th>IP</th><th>SSH Key</th></tr></thead><tbody>';
    preview.forEach((line, i) => {
      const cols = line.split(",").map(c => c.trim());
      if (i === 0 && (cols[0].toLowerCase() === "name" || cols[0].toLowerCase() === "hostname")) return;
      html += `<tr><td>${esc(cols[0] || "")}</td><td>${esc(cols[1] || "")}</td><td>${esc((cols[2] || "").substring(0, 30))}${(cols[2] || "").length > 30 ? "…" : ""}</td></tr>`;
    });
    html += "</tbody></table>";
    if (lines.length > 6) html += `<p class="csv-upload__more">…and ${lines.length - 6} more rows</p>`;
    document.getElementById("csv-preview-table").innerHTML = html;
    document.getElementById("csv-preview").style.display = "block";
  };
  reader.readAsText(file);
}

async function handleCSVUpload() {
  if (!csvFile) return;
  const btn = document.getElementById("csv-submit");
  btn.disabled = true;
  btn.textContent = "Uploading...";

  try {
    const fd = new FormData();
    fd.append("file", csvFile);
    const result = await api.uploadCSV(fd);

    const div = document.getElementById("csv-result");
    let html = `<p class="csv-result__summary">✅ Added: <strong>${result.added}</strong> | ⏭ Skipped: <strong>${result.skipped}</strong></p>`;
    if (result.errors?.length) {
      html += '<ul class="csv-result__errors">';
      result.errors.forEach(e => html += `<li>${esc(e)}</li>`);
      html += "</ul>";
    }
    div.innerHTML = html;
    div.style.display = "block";

    if (result.added > 0) {
      computers = await api.list();
      renderAllCards();
      showToast(`${result.added} devices imported!`, "success");
    }
  } catch (e) {
    showToast(`Upload failed: ${e.message}`, "error");
  } finally {
    btn.disabled = false;
    btn.textContent = "Upload Devices";
  }
}

// ============================================================================
// Modals
// ============================================================================

function openAddModal() {
  document.getElementById("modal-title").textContent = "Add Device";
  document.getElementById("form-submit").textContent = "Add Device";
  document.getElementById("form-edit-id").value = "";
  document.getElementById("form-name").value = "";
  document.getElementById("form-ip").value = "";
  document.getElementById("form-sshkey").value = "";
  document.getElementById("modal").classList.add("modal--open");
}

function openEditModal(comp) {
  document.getElementById("modal-title").textContent = "Edit Device";
  document.getElementById("form-submit").textContent = "Save Changes";
  document.getElementById("form-edit-id").value = comp.id;
  document.getElementById("form-name").value = comp.name;
  document.getElementById("form-ip").value = comp.ip;
  document.getElementById("form-sshkey").value = comp.sshKey || "";
  document.getElementById("modal").classList.add("modal--open");
}

function closeModal() { document.getElementById("modal").classList.remove("modal--open"); }

function openCSVModal() {
  csvFile = null;
  document.getElementById("csv-filename").textContent = "";
  document.getElementById("csv-file").value = "";
  document.getElementById("csv-preview").style.display = "none";
  document.getElementById("csv-result").style.display = "none";
  document.getElementById("csv-submit").disabled = true;
  document.getElementById("csv-modal").classList.add("modal--open");
}

function closeCSVModal() { document.getElementById("csv-modal").classList.remove("modal--open"); }

function openDeleteModal(id, name) {
  deleteTargetId = id;
  document.getElementById("delete-msg").textContent = `Are you sure you want to delete "${name}"? This cannot be undone.`;
  document.getElementById("delete-modal").classList.add("modal--open");
}

function closeDeleteModal() {
  deleteTargetId = null;
  document.getElementById("delete-modal").classList.remove("modal--open");
}

// ============================================================================
// Utilities
// ============================================================================

function showToast(msg, type = "success") {
  const t = document.getElementById("toast");
  t.textContent = msg;
  t.className = `toast toast--${type} toast--show`;
  setTimeout(() => t.classList.remove("toast--show"), 3000);
}

function esc(text) {
  const d = document.createElement("div");
  d.textContent = text;
  return d.innerHTML;
}

// ============================================================================
// Init
// ============================================================================

async function init() {
  // Start visual effects immediately
  initParallax();
  initParticles();

  try {
    computers = await api.list();
    renderAllCards();
    document.getElementById("count-online").textContent = "0";
    document.getElementById("count-offline").textContent = "0";
    console.log(`✨ Loaded ${computers.length} devices`);
  } catch (e) {
    const err = document.getElementById("error");
    err.style.display = "block";
    err.textContent = "⚠️ Cannot reach server. Is the Go backend running?";
  }
}

function setupEventListeners() {
  // Header
  document.getElementById("btn-ping-all").addEventListener("click", handlePingAll);
  document.getElementById("btn-add").addEventListener("click", openAddModal);
  document.getElementById("btn-upload").addEventListener("click", openCSVModal);

  // Form modal
  document.getElementById("modal-close").addEventListener("click", closeModal);
  document.getElementById("modal").addEventListener("click", e => { if (e.target.id === "modal") closeModal(); });
  document.getElementById("device-form").addEventListener("submit", handleFormSubmit);

  // CSV modal
  document.getElementById("csv-modal-close").addEventListener("click", closeCSVModal);
  document.getElementById("csv-modal").addEventListener("click", e => { if (e.target.id === "csv-modal") closeCSVModal(); });
  document.getElementById("csv-file").addEventListener("change", e => handleCSVFileSelect(e.target.files[0]));
  document.getElementById("csv-submit").addEventListener("click", handleCSVUpload);

  // Drag & drop
  const drop = document.getElementById("csv-dropzone");
  drop.addEventListener("dragover", e => { e.preventDefault(); drop.classList.add("csv-upload__dropzone--active"); });
  drop.addEventListener("dragleave", () => drop.classList.remove("csv-upload__dropzone--active"));
  drop.addEventListener("drop", e => {
    e.preventDefault();
    drop.classList.remove("csv-upload__dropzone--active");
    if (e.dataTransfer.files.length) handleCSVFileSelect(e.dataTransfer.files[0]);
  });

  // Delete modal
  document.getElementById("delete-modal-close").addEventListener("click", closeDeleteModal);
  document.getElementById("delete-cancel").addEventListener("click", closeDeleteModal);
  document.getElementById("delete-confirm").addEventListener("click", handleDelete);
  document.getElementById("delete-modal").addEventListener("click", e => { if (e.target.id === "delete-modal") closeDeleteModal(); });
}

setupEventListeners();
init();
