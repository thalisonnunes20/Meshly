// State
let currentServer = null;
let currentNickname = "";
let currentRoom = null;
let isTunnelActive = true;
let clientPublicKey = "";
let savedRooms = [];

// Elements - Screen 1
const screenConnect = document.getElementById("screen-connect");
const screenNetwork = document.getElementById("screen-network");
const inputEndpoint = document.getElementById("server-endpoint");
const inputNickname = document.getElementById("client-nickname");
const btnConnect = document.getElementById("btn-connect");
const connectError = document.getElementById("connect-error");

// Elements - Screen 2
const topbarEndpoint = document.getElementById("topbar-endpoint");
const topbarNickname = document.getElementById("topbar-nickname");
const btnDisconnectServer = document.getElementById("btn-disconnect-server");

const btnPowerToggle = document.getElementById("btn-power-toggle");
const powerStatusText = document.getElementById("power-status-text");
const myVirtualIP = document.getElementById("my-virtual-ip");
const btnCopyMyIP = document.getElementById("btn-copy-my-ip");

const tabJoin = document.getElementById("tab-join");
const tabCreate = document.getElementById("tab-create");
const formJoin = document.getElementById("form-join-network");
const formCreate = document.getElementById("form-create-network");

const inputJoinName = document.getElementById("join-room-name");
const inputJoinPass = document.getElementById("join-room-pass");
const btnJoinSubmit = document.getElementById("btn-join-submit");
const joinError = document.getElementById("join-error");

const inputCreateName = document.getElementById("create-room-name");
const inputCreatePass = document.getElementById("create-room-pass");
const btnCreateSubmit = document.getElementById("btn-create-submit");
const createError = document.getElementById("create-error");

const activeRoomView = document.getElementById("active-room-view");
const emptyNetworkView = document.getElementById("empty-network-view");
const activeRoomName = document.getElementById("active-room-name");
const activeRoomSubnet = document.getElementById("active-room-subnet");
const btnLeaveRoom = document.getElementById("btn-leave-room");
const peersList = document.getElementById("peers-list");

const savedRoomsSection = document.getElementById("saved-rooms-section");
const savedRoomsList = document.getElementById("saved-rooms-list");
const footerPeerCount = document.getElementById("footer-peer-count");

// Initialize on Load (Session Restore)
window.addEventListener("DOMContentLoaded", async () => {
  try {
    if (window.go && window.go.main && window.go.main.App) {
      // Automatic session initialization
      const session = await window.go.main.App.InitSession();
      
      if (session) {
        inputEndpoint.value = session.endpoint || "192.168.10.2:8080";
        inputNickname.value = session.nickname || "";
        savedRooms = session.saved_rooms || [];

        if (session.is_connected && session.server_info) {
          currentServer = session.server_info;
          currentNickname = session.nickname;
          clientPublicKey = session.server_info.client_public_key;

          topbarEndpoint.textContent = session.endpoint.replace(/https?:\/\//, "");
          topbarNickname.textContent = session.nickname;

          screenConnect.classList.remove("active");
          screenNetwork.classList.add("active");

          if (session.active_room) {
            renderRoom(session.active_room);
          } else {
            renderSavedRooms();
          }
        }
      }
    }
  } catch (err) {
    console.error("Session restore failed:", err);
  }

  // WebSocket / Runtime Event listener
  if (window.runtime && window.runtime.EventsOn) {
    window.runtime.EventsOn("room_event", (event) => {
      handleRoomEvent(event);
    });
  }
});

// ==========================================
// TELA 1: CONECTAR AO SERVIDOR
// ==========================================
btnConnect.addEventListener("click", async () => {
  const endpoint = inputEndpoint.value.trim();
  const nickname = inputNickname.value.trim();

  hideError(connectError);

  if (!endpoint) {
    showError(connectError, "Informe o endereço do servidor.");
    return;
  }
  if (!nickname) {
    showError(connectError, "Informe um apelido para a rede.");
    return;
  }

  btnConnect.disabled = true;
  btnConnect.querySelector(".btn-text").textContent = "Conectando...";

  try {
    const health = await window.go.main.App.ConnectServer(endpoint, nickname);
    currentServer = health;
    currentNickname = nickname;
    clientPublicKey = health.client_public_key;

    topbarEndpoint.textContent = endpoint.replace(/https?:\/\//, "");
    topbarNickname.textContent = nickname;

    screenConnect.classList.remove("active");
    screenNetwork.classList.add("active");

    const cfg = await window.go.main.App.GetStoredConfig();
    savedRooms = cfg.saved_rooms || [];
    renderSavedRooms();
  } catch (err) {
    showError(connectError, err.message || err.toString());
  } finally {
    btnConnect.disabled = false;
    btnConnect.querySelector(".btn-text").textContent = "Conectar ao Servidor";
  }
});

btnDisconnectServer.addEventListener("click", async () => {
  if (currentRoom) {
    await window.go.main.App.LeaveRoom();
    currentRoom = null;
  }
  screenNetwork.classList.remove("active");
  screenConnect.classList.add("active");
});

// ==========================================
// TABS: ENTRAR / CRIAR REDE
// ==========================================
tabJoin.addEventListener("click", () => {
  tabJoin.classList.add("active");
  tabCreate.classList.remove("active");
  formJoin.classList.remove("hidden");
  formCreate.classList.add("hidden");
  hideError(joinError);
  hideError(createError);
});

tabCreate.addEventListener("click", () => {
  tabCreate.classList.add("active");
  tabJoin.classList.remove("active");
  formCreate.classList.remove("hidden");
  formJoin.classList.add("hidden");
  hideError(joinError);
  hideError(createError);
});

// ==========================================
// CRIAR NOVA REDE
// ==========================================
btnCreateSubmit.addEventListener("click", async () => {
  const name = inputCreateName.value.trim();
  const pass = inputCreatePass.value.trim();

  hideError(createError);
  if (!name || pass.length < 3) {
    showError(createError, "Nome e senha (mín. 3 caracteres) são obrigatórios.");
    return;
  }

  btnCreateSubmit.disabled = true;
  try {
    await window.go.main.App.CreateRoom(name, pass);
    const roomState = await window.go.main.App.JoinRoom(name, pass);
    renderRoom(roomState);
    inputCreateName.value = "";
    inputCreatePass.value = "";
  } catch (err) {
    showError(createError, err.message || err.toString());
  } finally {
    btnCreateSubmit.disabled = false;
  }
});

// ==========================================
// ENTRAR EM REDE EXISTENTE
// ==========================================
btnJoinSubmit.addEventListener("click", async () => {
  const name = inputJoinName.value.trim();
  const pass = inputJoinPass.value.trim();

  hideError(joinError);
  if (!name || !pass) {
    showError(joinError, "Informe o nome e a senha da rede.");
    return;
  }

  btnJoinSubmit.disabled = true;
  try {
    const roomState = await window.go.main.App.JoinRoom(name, pass);
    renderRoom(roomState);
    inputJoinName.value = "";
    inputJoinPass.value = "";
  } catch (err) {
    showError(joinError, err.message || err.toString());
  } finally {
    btnJoinSubmit.disabled = false;
  }
});

// ==========================================
// DESCONECTAR DA SALA ATUAL
// ==========================================
btnLeaveRoom.addEventListener("click", async () => {
  try {
    await window.go.main.App.LeaveRoom();
    currentRoom = null;
    activeRoomView.classList.add("hidden");
    emptyNetworkView.classList.remove("hidden");
    myVirtualIP.textContent = "---.---.---.---";
    footerPeerCount.textContent = "0 membros";

    const cfg = await window.go.main.App.GetStoredConfig();
    savedRooms = cfg.saved_rooms || [];
    renderSavedRooms();
  } catch (err) {
    console.error("Error leaving room:", err);
  }
});

// ==========================================
// RENDERIZAÇÃO DA SALA E PEERS
// ==========================================
function renderRoom(roomState) {
  currentRoom = roomState;

  emptyNetworkView.classList.add("hidden");
  savedRoomsSection.classList.add("hidden");
  activeRoomView.classList.remove("hidden");

  activeRoomName.textContent = roomState.room_name;
  activeRoomSubnet.textContent = roomState.subnet_cidr;
  myVirtualIP.textContent = roomState.virtual_ip;

  renderPeers(roomState.peers || []);
}

function renderPeers(peers) {
  peersList.innerHTML = "";
  footerPeerCount.textContent = `${peers.length} membros`;

  peers.forEach((peer) => {
    const isSelf = peer.public_key === clientPublicKey;

    const item = document.createElement("div");
    item.className = "peer-item";

    item.innerHTML = `
      <div class="peer-left">
        <span class="status-dot ${peer.is_online ? 'online' : ''}"></span>
        <div class="peer-info">
          <span class="peer-nickname">
            ${escapeHTML(peer.nickname)}
            ${isSelf ? '<span class="self-tag">Você</span>' : ''}
          </span>
          <span class="peer-ip">${peer.virtual_ip}</span>
        </div>
      </div>
      <div class="peer-right">
        <span class="peer-ping" id="ping-${peer.virtual_ip}">-- ms</span>
        <button class="btn-copy" onclick="copyText('${peer.virtual_ip}', this)">
          <span class="copy-text">Copiar</span>
        </button>
      </div>
    `;

    peersList.appendChild(item);

    // Silent background ping
    if (!isSelf && window.go && window.go.main && window.go.main.App) {
      setTimeout(async () => {
        try {
          const ms = await window.go.main.App.PingPeer(peer.virtual_ip);
          const pingEl = document.getElementById(`ping-${peer.virtual_ip}`);
          if (pingEl) {
            pingEl.textContent = ms >= 0 ? `${ms} ms` : "timeout";
          }
        } catch (e) {}
      }, 300);
    }
  });
}

function renderSavedRooms() {
  if (!savedRooms || savedRooms.length === 0) {
    savedRoomsSection.classList.add("hidden");
    return;
  }

  savedRoomsSection.classList.remove("hidden");
  savedRoomsList.innerHTML = savedRooms.map(r => `
    <div class="saved-room-item">
      <span class="saved-name">${escapeHTML(r.name)}</span>
      <div class="saved-actions">
        <button class="btn btn-sm btn-primary" onclick="reconnectRoom('${escapeHTML(r.name)}', '${escapeHTML(r.password)}')">Conectar</button>
        <button class="btn-subtle-danger" onclick="forgetRoom('${escapeHTML(r.name)}')">Remover</button>
      </div>
    </div>
  `).join('');
}

async function reconnectRoom(name, pass) {
  try {
    const roomState = await window.go.main.App.JoinRoom(name, pass);
    renderRoom(roomState);
  } catch (err) {
    alert("Erro ao reconectar na rede: " + err);
  }
}

async function forgetRoom(name) {
  if (confirm(`Remover a rede "${name}" das suas redes salvas?`)) {
    const cfg = await window.go.main.App.ForgetRoom(name);
    savedRooms = cfg.saved_rooms || [];
    renderSavedRooms();
  }
}

// Power Toggle
btnPowerToggle.addEventListener("click", async () => {
  isTunnelActive = !isTunnelActive;

  if (window.go && window.go.main && window.go.main.App) {
    await window.go.main.App.ToggleTunnel(isTunnelActive);
  }

  if (isTunnelActive) {
    btnPowerToggle.classList.add("active");
    powerStatusText.textContent = "Conectado";
  } else {
    btnPowerToggle.classList.remove("active");
    powerStatusText.textContent = "Desconectado";
  }
});

btnCopyMyIP.addEventListener("click", () => {
  const ip = myVirtualIP.textContent;
  if (ip && !ip.includes("-")) {
    copyText(ip, btnCopyMyIP);
  }
});

function handleRoomEvent(event) {
  if (!currentRoom || event.room_id !== currentRoom.room_id) return;

  if (event.type === "peer_joined" || event.type === "peer_left" || event.type === "peer_status") {
    if (window.go && window.go.main && window.go.main.App) {
      window.go.main.App.GetActiveRoom().then((room) => {
        if (room && room.peers) {
          renderPeers(room.peers);
        }
      });
    }
  }
}

// Helpers
function copyText(text, btnElement) {
  navigator.clipboard.writeText(text);
  const textSpan = btnElement.querySelector(".copy-text");
  if (textSpan) {
    const original = textSpan.textContent;
    textSpan.textContent = "Copiado";
    btnElement.classList.add("copied");
    setTimeout(() => {
      textSpan.textContent = original;
      btnElement.classList.remove("copied");
    }, 1500);
  }
}

function showError(el, message) {
  el.textContent = message;
  el.classList.remove("hidden");
}

function hideError(el) {
  el.textContent = "";
  el.classList.add("hidden");
}

function escapeHTML(str) {
  return str.replace(/[&<>'"]/g, 
    tag => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&quot;' }[tag] || tag)
  );
}
