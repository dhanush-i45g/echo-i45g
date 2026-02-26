/**
 * ============================================================================
 * Network PC Monitoring System - Frontend Application
 * ============================================================================
 * Client-side application for monitoring computer status on a network.
 * Provides UI for checking individual or all computers, and adding new ones.
 * ============================================================================
 */

"use strict";

// ============================================================================
// Configuration & State
// ============================================================================

const API = "/api";
let computers = [];
let deleteTargetId = null;

// ============================================================================
// API Service Layer
// ============================================================================

async function apiCall(url, options = {}) {
  const res = await fetch(url, options);
  const json = await res.json();
  if (!json.success) throw new Error(json.error);
  return json.data;
}

const api = {
  list: () => apiCall(`${API}/computers`),
  add: (data) => apiCall(`${API}/computers`, {
    method: "POST", headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  }),
  update: (id, data) => apiCall(`${API}/computers/${id}`, {
    method: "PUT", headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  }),
  remove: (id) => apiCall(`${API}/computers/${id}`, { method: "DELETE" }),
  ping: (id) => apiCall(`${API}/ping/${id}`),
  pingAll: () => apiCall(`${API}/ping-all`),
  uploadCSV: (formData) => apiCall(`${API}/computers/upload`, {
    method: "POST", body: formData,
  }),
};

// ============================================================================
// UI Rendering
// ============================================================================

function renderCard(computer, statusData = null) {
  const cardElement = document.getElementById(`card-${computer.id}`);
  const status = statusData ? statusData.status : null;
  const checkedAt = statusData ? statusData.checkedAt : null;
  const statusClass = status === "ON" ? "card--on" : status === "OFF" ? "card--off" : "card--idle";
  const badgeText = status || "UNKNOWN";
  const timeText = checkedAt ? `Checked: ${checkedAt}` : "Not checked yet";
  const hasKey = computer.sshKey && computer.sshKey.length > 0;

  const cardHTML = `
    <div class="card__indicator"><span class="card__dot"></span></div>
    <div class="card__body">
      <h2 class="card__name">${escapeHTML(computer.name)}</h2>
      <p class="card__ip">${escapeHTML(computer.ip)} ${hasKey ? '<span class="card__key-badge" title="SSH Key configured">🔑</span>' : ''}</p>
    </div>
    <div class="card__status">
      <span class="card__badge">${badgeText}</span>
    </div>
    <div class="card__footer">
      <span class="card__time">${timeText}</span>
      <div class="card__actions">
        <button class="btn-action btn-action--edit" data-id="${computer.id}" title="Edit">
          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
            <path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/>
            <path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/>
          </svg>
        </button>
        <button class="btn-action btn-action--delete" data-id="${computer.id}" title="Delete">
          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
            <polyline points="3 6 5 6 21 6"/>
            <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
          </svg>
        </button>
        <button class="btn-check" data-id="${computer.id}">
          <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
            <circle cx="12" cy="12" r="10"/>
            <polyline points="12 6 12 12 16 14"/>
          </svg>
          Check
        </button>
      </div>
    </div>
  `;

  if (cardElement) {
    cardElement.className = `card ${statusClass}`;
    cardElement.innerHTML = cardHTML;
  } else {
    const newCard = document.createElement("div");
    newCard.className = `card ${statusClass}`;
    newCard.id = `card-${computer.id}`;
    newCard.innerHTML = cardHTML;
    document.getElementById("grid").appendChild(newCard);
  }

  // Attach event listeners
  const card = document.getElementById(`card-${computer.id}`);
  card.querySelector(".btn-check").addEventListener("click", () => handlePingOne(computer.id));
  card.querySelector(".btn-action--edit").addEventListener("click", () => openEditModal(computer));
  card.querySelector(".btn-action--delete").addEventListener("click", () => openDeleteModal(computer.id, computer.name));
}

function setCardLoading(id, loading) {
  const button = document.querySelector(`#card-${id} .btn-check`);
  if (!button) return;
  button.disabled = loading;
  button.innerHTML = loading
    ? `<span class="spinner"></span> Checking...`
    : `<svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5">
        <circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/>
      </svg> Check`;
}

function updateSummary(results) {
  const on = results.filter((r) => r.status === "ON").length;
  const off = results.filter((r) => r.status === "OFF").length;
  document.getElementById("count-total").textContent = computers.length;
  document.getElementById("count-online").textContent = on;
  document.getElementById("count-offline").textContent = off;
}

function renderAllCards() {
  document.getElementById("grid").innerHTML = "";
  computers.forEach((c) => renderCard(c));
  document.getElementById("count-total").textContent = computers.length;
}

// ============================================================================
// Event Handlers - Ping
// ============================================================================

async function handlePingOne(id) {
  setCardLoading(id, true);
  try {
    const result = await api.ping(id);
    const computer = computers.find((c) => c.id === id);
    if (computer) renderCard(computer, result);
  } catch (error) {
    showToast(`Error checking ${id}: ${error.message}`, "error");
  } finally {
    setCardLoading(id, false);
  }
}

async function handlePingAll() {
  const button = document.getElementById("btn-ping-all");
  button.disabled = true;
  button.innerHTML = `<span class="spinner"></span> Checking all...`;
  try {
    const results = await api.pingAll();
    results.forEach((result) => {
      const computer = computers.find((c) => c.id === result.id);
      if (computer) renderCard(computer, result);
    });
    updateSummary(results);
    showToast("All devices checked!", "success");
  } catch (error) {
    showToast(`Error: ${error.message}`, "error");
  } finally {
    button.disabled = false;
    button.innerHTML = `Check All`;
  }
}

// ============================================================================
// Event Handlers - CRUD
// ============================================================================

async function handleFormSubmit(event) {
  event.preventDefault();
  const editId = document.getElementById("form-edit-id").value;
  const name = document.getElementById("form-name").value.trim();
  const ip = document.getElementById("form-ip").value.trim();
  const sshKey = document.getElementById("form-sshkey").value.trim();

  if (!name || !ip) {
    showToast("Name and IP are required", "error");
    return;
  }

  const submitBtn = document.getElementById("form-submit");
  submitBtn.disabled = true;
  submitBtn.textContent = editId ? "Saving..." : "Adding...";

  try {
    if (editId) {
      // Update existing
      const updated = await api.update(editId, { name, ip, sshKey });
      const idx = computers.findIndex((c) => c.id === parseInt(editId));
      if (idx !== -1) {
        computers[idx] = updated;
        renderCard(updated);
      }
      showToast(`${updated.name} updated!`, "success");
    } else {
      // Add new
      const newComputer = await api.add({ name, ip, sshKey });
      computers.push(newComputer);
      renderCard(newComputer);
      document.getElementById("count-total").textContent = computers.length;
      showToast(`${newComputer.name} added!`, "success");
    }
    closeModal();
  } catch (error) {
    showToast(`Error: ${error.message}`, "error");
  } finally {
    submitBtn.disabled = false;
    submitBtn.textContent = editId ? "Save Changes" : "Add Device";
  }
}

async function handleDelete() {
  if (!deleteTargetId) return;
  const btn = document.getElementById("delete-confirm");
  btn.disabled = true;
  btn.textContent = "Deleting...";

  try {
    await api.remove(deleteTargetId);
    computers = computers.filter((c) => c.id !== deleteTargetId);
    const cardEl = document.getElementById(`card-${deleteTargetId}`);
    if (cardEl) cardEl.remove();
    document.getElementById("count-total").textContent = computers.length;
    showToast("Device deleted", "success");
    closeDeleteModal();
  } catch (error) {
    showToast(`Error: ${error.message}`, "error");
  } finally {
    btn.disabled = false;
    btn.textContent = "Delete";
  }
}

// ============================================================================
// Event Handlers - CSV Upload
// ============================================================================

let csvFile = null;

function handleCSVFileSelect(file) {
  if (!file || !file.name.endsWith(".csv")) {
    showToast("Please select a .csv file", "error");
    return;
  }
  csvFile = file;
  document.getElementById("csv-filename").textContent = file.name;
  document.getElementById("csv-submit").disabled = false;

  // Preview first 5 rows
  const reader = new FileReader();
  reader.onload = (e) => {
    const lines = e.target.result.split("\n").filter((l) => l.trim());
    const preview = lines.slice(0, 6);
    let html = '<table class="csv-table"><thead><tr><th>Name</th><th>IP</th><th>SSH Key</th></tr></thead><tbody>';
    preview.forEach((line, i) => {
      const cols = line.split(",").map((c) => c.trim());
      // Skip header row
      if (i === 0 && (cols[0].toLowerCase() === "name" || cols[0].toLowerCase() === "hostname")) return;
      html += `<tr><td>${escapeHTML(cols[0] || "")}</td><td>${escapeHTML(cols[1] || "")}</td><td>${escapeHTML((cols[2] || "").substring(0, 30))}${(cols[2] || "").length > 30 ? "..." : ""}</td></tr>`;
    });
    html += "</tbody></table>";
    if (lines.length > 6) html += `<p class="csv-upload__more">...and ${lines.length - 6} more rows</p>`;
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
    const formData = new FormData();
    formData.append("file", csvFile);
    const result = await api.uploadCSV(formData);

    // Show result
    const resultDiv = document.getElementById("csv-result");
    let html = `<p class="csv-result__summary">✅ Added: <strong>${result.added}</strong> | ⏭ Skipped: <strong>${result.skipped}</strong></p>`;
    if (result.errors && result.errors.length > 0) {
      html += '<ul class="csv-result__errors">';
      result.errors.forEach((e) => (html += `<li>${escapeHTML(e)}</li>`));
      html += "</ul>";
    }
    resultDiv.innerHTML = html;
    resultDiv.style.display = "block";

    // Reload the computer list
    if (result.added > 0) {
      computers = await api.list();
      renderAllCards();
      showToast(`${result.added} devices imported!`, "success");
    }
  } catch (error) {
    showToast(`Upload failed: ${error.message}`, "error");
  } finally {
    btn.disabled = false;
    btn.textContent = "Upload Devices";
  }
}

// ============================================================================
// Modal Management
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

function openEditModal(computer) {
  document.getElementById("modal-title").textContent = "Edit Device";
  document.getElementById("form-submit").textContent = "Save Changes";
  document.getElementById("form-edit-id").value = computer.id;
  document.getElementById("form-name").value = computer.name;
  document.getElementById("form-ip").value = computer.ip;
  document.getElementById("form-sshkey").value = computer.sshKey || "";
  document.getElementById("modal").classList.add("modal--open");
}

function closeModal() {
  document.getElementById("modal").classList.remove("modal--open");
}

function openCSVModal() {
  csvFile = null;
  document.getElementById("csv-filename").textContent = "";
  document.getElementById("csv-file").value = "";
  document.getElementById("csv-preview").style.display = "none";
  document.getElementById("csv-result").style.display = "none";
  document.getElementById("csv-submit").disabled = true;
  document.getElementById("csv-modal").classList.add("modal--open");
}

function closeCSVModal() {
  document.getElementById("csv-modal").classList.remove("modal--open");
}

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
// Notifications & Utilities
// ============================================================================

function showToast(message, type = "success") {
  const toast = document.getElementById("toast");
  toast.textContent = message;
  toast.className = `toast toast--${type} toast--show`;
  setTimeout(() => toast.classList.remove("toast--show"), 3000);
}

function escapeHTML(text) {
  const div = document.createElement("div");
  div.textContent = text;
  return div.innerHTML;
}

// ============================================================================
// Initialization
// ============================================================================

async function init() {
  try {
    computers = await api.list();
    renderAllCards();
    document.getElementById("count-online").textContent = "0";
    document.getElementById("count-offline").textContent = "0";
    console.log(`Loaded ${computers.length} devices`);
  } catch (error) {
    const errorDiv = document.getElementById("error");
    errorDiv.style.display = "block";
    errorDiv.textContent = "⚠️ Cannot reach server. Is the Go server running?";
  }
}

function setupEventListeners() {
  // Header buttons
  document.getElementById("btn-ping-all").addEventListener("click", handlePingAll);
  document.getElementById("btn-add").addEventListener("click", openAddModal);
  document.getElementById("btn-upload").addEventListener("click", openCSVModal);

  // Device form modal
  document.getElementById("modal-close").addEventListener("click", closeModal);
  document.getElementById("modal").addEventListener("click", (e) => { if (e.target.id === "modal") closeModal(); });
  document.getElementById("device-form").addEventListener("submit", handleFormSubmit);

  // CSV modal
  document.getElementById("csv-modal-close").addEventListener("click", closeCSVModal);
  document.getElementById("csv-modal").addEventListener("click", (e) => { if (e.target.id === "csv-modal") closeCSVModal(); });
  document.getElementById("csv-file").addEventListener("change", (e) => handleCSVFileSelect(e.target.files[0]));
  document.getElementById("csv-submit").addEventListener("click", handleCSVUpload);

  // CSV drag & drop
  const dropzone = document.getElementById("csv-dropzone");
  dropzone.addEventListener("dragover", (e) => { e.preventDefault(); dropzone.classList.add("csv-upload__dropzone--active"); });
  dropzone.addEventListener("dragleave", () => dropzone.classList.remove("csv-upload__dropzone--active"));
  dropzone.addEventListener("drop", (e) => {
    e.preventDefault();
    dropzone.classList.remove("csv-upload__dropzone--active");
    if (e.dataTransfer.files.length) handleCSVFileSelect(e.dataTransfer.files[0]);
  });

  // Delete modal
  document.getElementById("delete-modal-close").addEventListener("click", closeDeleteModal);
  document.getElementById("delete-cancel").addEventListener("click", closeDeleteModal);
  document.getElementById("delete-confirm").addEventListener("click", handleDelete);
  document.getElementById("delete-modal").addEventListener("click", (e) => { if (e.target.id === "delete-modal") closeDeleteModal(); });
}

setupEventListeners();
init();
