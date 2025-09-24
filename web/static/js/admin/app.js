const state = {
    rooms: [],
    roomsMap: new Map(),
    filteredRooms: [],
    currentRoomId: null,
    agentId: null,
    agentDisplayName: "客服小幫手",
    account: null,
    token: null,
    onlineAgents: [],
    socket: null,
    typingTimer: null,
    typingBubble: null,
    isSidebarCollapsed: false,
    searchKeyword: "",
    roomsInterval: null,
    agentsInterval: null,
    agencySettings: [],
};

const dom = {
    sidebarAgentName: document.getElementById("sidebarAgentName"),
    sidebarAgentRole: document.getElementById("sidebarAgentRole"),
    connectionBadge: document.getElementById("connectionBadge"),
    refreshRooms: document.getElementById("refreshRooms"),
    roomList: document.getElementById("roomList"),
    roomTitle: document.getElementById("roomTitle"),
    roomMeta: document.getElementById("roomMeta"),
    roomSearch: document.getElementById("roomSearch"),
    roomCounter: document.getElementById("listCounter"),
    messageStream: document.getElementById("messageStream"),
    messageForm: document.getElementById("messageForm"),
    messageInput: document.getElementById("messageInput"),
    sendButton: document.getElementById("sendMessage"),
    sendTyping: document.getElementById("sendTyping"),
    agentName: document.getElementById("agentName"),
    assignAgent: document.getElementById("assignAgent"),
    waitingCount: document.getElementById("waitingCount"),
    activeCount: document.getElementById("activeCount"),
    agentOnlineCount: document.getElementById("agentOnlineCount"),
    workspace: document.querySelector(".workspace"),
    workspaceList: document.querySelector(".workspace-list"),
    toggleSidebar: document.getElementById("toggleSidebar"),
    metricsDrawer: document.getElementById("metricsDrawer"),
    openMetrics: document.getElementById("openMetrics"),
    closeMetrics: document.getElementById("closeMetrics"),
    drawerMetrics: document.getElementById("drawerMetrics"),
    metricsPanel: document.getElementById("metricsPanel"),
    authOverlay: document.getElementById("authOverlay"),
    loginForm: document.getElementById("loginForm"),
    loginUsername: document.getElementById("loginUsername"),
    loginPassword: document.getElementById("loginPassword"),
    loginError: document.getElementById("loginError"),
    logoutButton: document.getElementById("logoutButton"),
    openAccountDrawer: document.getElementById("openAccountDrawer"),
    accountDrawer: document.getElementById("accountDrawer"),
    closeAccountDrawer: document.getElementById("closeAccountDrawer"),
    accountSummary: document.getElementById("accountSummary"),
    accountMessage: document.getElementById("accountMessage"),
    createAgentSection: document.getElementById("createAgentSection"),
    createAgentForm: document.getElementById("createAgentForm"),
    agentUsername: document.getElementById("agentUsername"),
    agentPassword: document.getElementById("agentPassword"),
    agentDisplay: document.getElementById("agentDisplay"),
    agentAgency: document.getElementById("agentAgency"),
    createPlayerForm: document.getElementById("createPlayerForm"),
    playerUsername: document.getElementById("playerUsername"),
    playerPassword: document.getElementById("playerPassword"),
    playerDisplay: document.getElementById("playerDisplay"),
    playerAgency: document.getElementById("playerAgency"),
    onlineAgentList: document.getElementById("onlineAgentList"),
    headerAvatar: document.getElementById("headerAvatar"),
    transferTarget: document.getElementById("transferTarget"),
    transferAgent: document.getElementById("transferAgent"),
    openSidebarHandle: document.getElementById("openSidebarHandle"),
    agencySettingsSection: document.getElementById("agencySettingsSection"),
    agencySettingsForm: document.getElementById("agencySettingsForm"),
    settingsAgency: document.getElementById("settingsAgency"),
    settingsSelect: document.getElementById("settingsSelect"),
    settingsCharge: document.getElementById("settingsCharge"),
    settingsWithdraw: document.getElementById("settingsWithdraw"),
    settingsBet: document.getElementById("settingsBet"),
    settingsPlayerInfo: document.getElementById("settingsPlayerInfo"),
    resetAgencySettings: document.getElementById("resetAgencySettings"),
    agencySettingsMessage: document.getElementById("agencySettingsMessage"),
};

const REFRESH_ROOMS_INTERVAL = 30000;
const ACTIVE_THRESHOLD_MS = 5 * 60 * 1000;
const TYPING_TIMEOUT = 1500;
const ONLINE_AGENTS_INTERVAL = 20000;

function parseDate(value) {
    if (!value) return null;
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? null : date;
}

function formatClock(value) {
    const date = typeof value === "string" ? parseDate(value) : value;
    if (!date) return "--:--";
    return date.toLocaleTimeString("zh-TW", { hour: "2-digit", minute: "2-digit" });
}

function formatRelative(value) {
    const date = typeof value === "string" ? parseDate(value) : value;
    if (!date) return "-";
    const diff = Date.now() - date.getTime();
    if (diff < 60 * 1000) return "剛剛";
    if (diff < 60 * 60 * 1000) return `${Math.floor(diff / (60 * 1000))} 分鐘前`;
    if (diff < 24 * 60 * 60 * 1000) return `${Math.floor(diff / (60 * 60 * 1000))} 小時前`;
    return date.toLocaleDateString("zh-TW", { month: "2-digit", day: "2-digit" });
}

function formatDayLabel(date) {
    const parsed = typeof date === "string" ? parseDate(date) : date;
    if (!parsed) return "";
    const base = new Date();
    base.setHours(0, 0, 0, 0);
    const compare = new Date(parsed);
    compare.setHours(0, 0, 0, 0);
    const diffDays = Math.round((base - compare) / (24 * 60 * 60 * 1000));
    if (diffDays === 0) return "今天";
    if (diffDays === 1) return "昨天";
    if (diffDays === -1) return "明天";
    return parsed.toLocaleDateString("zh-TW", { month: "2-digit", day: "2-digit", weekday: "short" });
}

function getCmd(message) {
    return message.cmd || message.type || "";
}

function apiFetch(url, options = {}) {
    const init = { ...options };
    const headers = new Headers(options.headers || {});
    if (state.token) {
        headers.set("Authorization", `Bearer ${state.token}`);
    }
    init.headers = headers;
    return fetch(url, init);
}

function showAuthOverlay(message = "") {
    if (dom.loginError) {
        dom.loginError.textContent = message;
    }
    if (dom.authOverlay) {
        dom.authOverlay.hidden = false;
    }
    if (dom.loginPassword) {
        dom.loginPassword.value = "";
    }
}

function hideAuthOverlay() {
    if (dom.authOverlay) {
        dom.authOverlay.hidden = true;
    }
    if (dom.loginError) {
        dom.loginError.textContent = "";
    }
}

function persistToken(token) {
    if (token) {
        localStorage.setItem("imAdminToken", token);
    } else {
        localStorage.removeItem("imAdminToken");
    }
}

function updateAvatar(displayName) {
    if (!dom.headerAvatar) return;
    const initial = displayName ? displayName.trim().charAt(0).toUpperCase() : "IM";
    dom.headerAvatar.textContent = initial || "IM";
}

function displayAccountMessage(text, type = "info") {
    if (!dom.accountMessage) return;
    dom.accountMessage.textContent = text;
    dom.accountMessage.dataset.type = type;
}

function displayAgencySettingsMessage(text, type = "info") {
    if (!dom.agencySettingsMessage) return;
    dom.agencySettingsMessage.textContent = text;
    dom.agencySettingsMessage.dataset.type = type;
}

function resetAgencySettingsForm() {
    if (dom.settingsSelect) {
        dom.settingsSelect.value = "";
    }
    if (dom.settingsAgency) {
        dom.settingsAgency.value = "";
    }
    if (dom.settingsCharge) {
        dom.settingsCharge.value = "";
    }
    if (dom.settingsWithdraw) {
        dom.settingsWithdraw.value = "";
    }
    if (dom.settingsBet) {
        dom.settingsBet.value = "";
    }
    if (dom.settingsPlayerInfo) {
        dom.settingsPlayerInfo.value = "";
    }
}

function updateAgencySettingsOptions() {
    if (!dom.settingsSelect) return;
    const options = ["<option value=\"\">選擇代理或輸入新代理</option>"];
    state.agencySettings.sort((a, b) => a.agency.localeCompare(b.agency)).forEach((item) => {
        const escaped = item.agency.replace(/"/g, "&quot;");
        options.push(`<option value="${escaped}">${escaped}</option>`);
    });
    dom.settingsSelect.innerHTML = options.join("");
}

function fillAgencySettingsForm(agency) {
    if (!dom.settingsAgency) return;
    const normalized = (agency || "").trim();
    const target = state.agencySettings.find((item) => item.agency === normalized);
    dom.settingsAgency.value = normalized;
    if (dom.settingsCharge) dom.settingsCharge.value = target?.chargeApi || "";
    if (dom.settingsWithdraw) dom.settingsWithdraw.value = target?.withdrawApi || "";
    if (dom.settingsBet) dom.settingsBet.value = target?.betApi || "";
    if (dom.settingsPlayerInfo) dom.settingsPlayerInfo.value = target?.playerInfoApi || "";
    if (target && target.updatedAt) {
        const updatedTime = new Date(target.updatedAt);
        const label = Number.isNaN(updatedTime.getTime()) ? "" : updatedTime.toLocaleString("zh-TW");
        displayAgencySettingsMessage(label ? `已更新：${label}` : "已載入代理設定", "success");
    } else {
        displayAgencySettingsMessage("可填寫 API 端點後儲存", "info");
    }
}

async function loadAgencySettingsList({ silent = false } = {}) {
    if (!state.token || !dom.agencySettingsSection || dom.agencySettingsSection.hidden) {
        return;
    }
    try {
        const response = await apiFetch("/api/agencies/settings");
        if (!response.ok) {
            if (response.status === 401) {
                handleUnauthorized();
                return;
            }
            if (!silent) {
                const message = await response.text();
                displayAgencySettingsMessage(message || "載入代理設定失敗", "error");
            }
            return;
        }
        const list = await response.json();
        state.agencySettings = Array.isArray(list) ? list : [];
        updateAgencySettingsOptions();
    } catch (error) {
        console.error("load agency settings failed", error);
        if (!silent) {
            displayAgencySettingsMessage("載入代理設定失敗，請稍後再試", "error");
        }
    }
}

async function saveAgencySettings(event) {
    event.preventDefault();
    if (!state.token) {
        showAuthOverlay();
        return;
    }
    const agencyValue = (dom.settingsAgency?.value || "").trim().toLowerCase() || (dom.settingsSelect?.value || "").trim().toLowerCase();
    if (!agencyValue) {
        displayAgencySettingsMessage("請輸入代理代碼", "error");
        return;
    }
    const payload = {
        chargeApi: dom.settingsCharge ? dom.settingsCharge.value.trim() : "",
        withdrawApi: dom.settingsWithdraw ? dom.settingsWithdraw.value.trim() : "",
        betApi: dom.settingsBet ? dom.settingsBet.value.trim() : "",
        playerInfoApi: dom.settingsPlayerInfo ? dom.settingsPlayerInfo.value.trim() : "",
    };
    try {
        const response = await apiFetch(`/api/agencies/settings/${encodeURIComponent(agencyValue)}`, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(payload),
        });
        if (!response.ok) {
            if (response.status === 401) {
                handleUnauthorized();
                return;
            }
            const text = await response.text();
            displayAgencySettingsMessage(text || "儲存代理設定失敗", "error");
            return;
        }
        const data = await response.json();
        const existingIndex = state.agencySettings.findIndex((item) => item.agency === data.agency);
        if (existingIndex >= 0) {
            state.agencySettings[existingIndex] = data;
        } else {
            state.agencySettings.push(data);
        }
        updateAgencySettingsOptions();
        if (dom.settingsSelect) {
            dom.settingsSelect.value = data.agency;
        }
        fillAgencySettingsForm(data.agency);
        displayAgencySettingsMessage("代理設定已儲存", "success");
    } catch (error) {
        console.error("save agency settings failed", error);
        displayAgencySettingsMessage("儲存代理設定失敗，請稍後再試", "error");
    }
}

function updateAccountControls() {
    if (dom.createAgentSection) {
        const shouldShow = state.account && state.account.username === "admin01";
        dom.createAgentSection.hidden = !shouldShow;
    }
    if (dom.agencySettingsSection) {
        const showAgencySettings = state.account && state.account.role === "admin";
        dom.agencySettingsSection.hidden = !showAgencySettings;
    }
}

function resetRoomView() {
    state.currentRoomId = null;
    dom.roomTitle.textContent = "請選擇對話";
    dom.roomMeta.textContent = "尚未選擇任何房間";
    if (dom.messageStream) {
        dom.messageStream.innerHTML = "<div class=\"empty-placeholder\">請從左側選擇一個房間以載入對話</div>";
    }
    enableComposer(false);
    setConnectionBadge(false);
}

function stopAutoRefresh() {
    if (state.roomsInterval) {
        clearInterval(state.roomsInterval);
        state.roomsInterval = null;
    }
    if (state.agentsInterval) {
        clearInterval(state.agentsInterval);
        state.agentsInterval = null;
    }
}

function clearSession({ keepOverlay = false, message = "" } = {}) {
    stopAutoRefresh();
    closeSocket();
    state.account = null;
    state.token = null;
    state.agentId = null;
    state.agentDisplayName = "客服小幫手";
    state.roomsMap.clear();
    state.rooms = [];
    state.filteredRooms = [];
    state.onlineAgents = [];
    state.agencySettings = [];
    renderRoomList();
    updateMetrics();
    resetRoomView();
    persistToken(null);
    dom.sidebarAgentName.textContent = "客服小幫手";
    if (dom.sidebarAgentRole) {
        dom.sidebarAgentRole.textContent = "尚未登入";
    }
    updateAvatar();
    dom.agentName.value = "";
    dom.agentName.disabled = true;
    dom.assignAgent.disabled = true;
    if (dom.transferTarget) {
        dom.transferTarget.disabled = true;
        dom.transferTarget.innerHTML = "<option value=\"\">轉接至在線客服</option>";
    }
    if (dom.transferAgent) {
        dom.transferAgent.disabled = true;
    }
    dom.logoutButton.disabled = true;
    displayAccountMessage("", "info");
    if (dom.accountSummary) {
        dom.accountSummary.textContent = "尚未登入";
    }
    if (dom.agencySettingsMessage) {
        dom.agencySettingsMessage.textContent = "";
        dom.agencySettingsMessage.dataset.type = "info";
    }
    resetAgencySettingsForm();
    updateAgencySettingsOptions();
    if (dom.onlineAgentList) {
        dom.onlineAgentList.innerHTML = "<li class=\"empty\">尚無在線客服</li>";
    }
    if (dom.accountDrawer) {
        dom.accountDrawer.hidden = true;
    }
    if (dom.metricsDrawer) {
        dom.metricsDrawer.hidden = true;
    }
    if (!keepOverlay) {
        showAuthOverlay(message);
    } else if (message && dom.loginError) {
        dom.loginError.textContent = message;
    }
    updateAccountControls();
}

function applySession(account, token) {
    state.account = account;
    if (token) {
        state.token = token;
        persistToken(token);
    }
    state.agentId = account.username;
    state.agentDisplayName = account.displayName || account.username;
    dom.sidebarAgentName.textContent = state.agentDisplayName;
    if (dom.sidebarAgentRole) {
        dom.sidebarAgentRole.textContent = account.role === "admin" ? "客服管理員" : "客服專員";
    }
    dom.agentName.value = state.agentDisplayName;
    dom.agentName.disabled = false;
    dom.assignAgent.disabled = false;
    dom.transferTarget.disabled = false;
    dom.logoutButton.disabled = false;
    if (dom.transferAgent) {
        dom.transferAgent.disabled = true;
    }
    updateAvatar(state.agentDisplayName);
    const roleLabel = account.role === "admin" ? "管理員" : "客服";
    displayAccountMessage(`登入帳號：${account.username}（${roleLabel}）`, "success");
    if (dom.accountSummary) {
        const agencyLabel = account.agency || "default";
        dom.accountSummary.textContent = `帳號：${account.username} · 代理：${agencyLabel} · 身分：${roleLabel}`;
    }
    updateAccountControls();
    loadAgencySettingsList({ silent: true });
    hideAuthOverlay();
}

function handleUnauthorized(message = "請重新登入") {
    clearSession({ keepOverlay: false, message });
}

function startAutoRefresh() {
    stopAutoRefresh();
    state.roomsInterval = setInterval(() => loadRooms({ silent: true }), REFRESH_ROOMS_INTERVAL);
    state.agentsInterval = setInterval(() => loadOnlineAgents(), ONLINE_AGENTS_INTERVAL);
}

async function onLogin(account, token) {
    applySession(account, token);
    await Promise.all([loadRooms(), loadOnlineAgents()]);
    startAutoRefresh();
}

async function registerAccount(role, username, password, displayName, agency = "") {
    if (!state.token) {
        showAuthOverlay();
        return null;
    }
    const normalizedDisplay = displayName || username;
    const trimmedAgency = agency.trim();
    try {
        const response = await apiFetch("/api/auth/register", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ username, password, displayName: normalizedDisplay, role, agency: trimmedAgency }),
        });
        if (!response.ok) {
            if (response.status === 401) {
                handleUnauthorized();
                return null;
            }
            const errorText = await response.text();
            displayAccountMessage(errorText || "建立帳號失敗", "error");
            return null;
        }
        const account = await response.json();
        return account;
    } catch (error) {
        console.error("register account failed", error);
        displayAccountMessage("建立帳號失敗，請稍後再試", "error");
        return null;
    }
}

async function handleLoginSubmit(event) {
    event.preventDefault();
    const username = dom.loginUsername.value.trim();
    const password = dom.loginPassword.value;
    if (!username || !password) {
        if (dom.loginError) dom.loginError.textContent = "請輸入帳號與密碼";
        return;
    }
    try {
        const response = await fetch("/api/auth/login", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ username, password }),
        });
        if (!response.ok) {
            const message = await response.text();
            if (dom.loginError) dom.loginError.textContent = message || "登入失敗";
            return;
        }
        const payload = await response.json();
        dom.loginUsername.value = "";
        dom.loginPassword.value = "";
        await onLogin(payload.account, payload.token);
    } catch (error) {
        console.error("login failed", error);
        if (dom.loginError) dom.loginError.textContent = "登入失敗，請稍後再試";
    }
}

async function logout() {
    if (state.token) {
        try {
            await apiFetch("/api/auth/logout", { method: "POST" });
        } catch (error) {
            console.warn("logout request failed", error);
        }
    }
    clearSession();
}

async function restoreSession() {
    const stored = localStorage.getItem("imAdminToken");
    if (!stored) {
        clearSession();
        return;
    }
    state.token = stored;
    try {
        const response = await apiFetch("/api/auth/profile");
        if (!response.ok) {
            clearSession();
            return;
        }
        const account = await response.json();
        await onLogin(account, stored);
    } catch (error) {
        console.error("restore session failed", error);
        clearSession();
    }
}

async function loadRooms({ silent = false } = {}) {
    try {
        if (!silent) {
            dom.roomList.classList.add("loading");
        }
        if (!state.token) {
            dom.roomList.classList.remove("loading");
            return;
        }
        const response = await apiFetch("/api/rooms");
        if (!response.ok) {
            if (response.status === 401) {
                handleUnauthorized();
                return;
            }
            throw new Error(await response.text());
        }
        const rooms = await response.json();
        state.roomsMap = new Map(rooms.map((room) => [room.roomId, room]));
        syncRooms();
        applyFilter();
        updateMetrics();
    } catch (error) {
        console.error("loadRooms failed", error);
    } finally {
        dom.roomList.classList.remove("loading");
    }
}

async function loadOnlineAgents() {
    if (!state.token) return;
    try {
        const response = await apiFetch("/api/agents/online");
        if (!response.ok) {
            if (response.status === 401) {
                handleUnauthorized();
                return;
            }
            throw new Error(await response.text());
        }
        const agents = await response.json();
        state.onlineAgents = Array.isArray(agents) ? agents : [];
        renderOnlineAgents();
        updateMetrics();
    } catch (error) {
        console.error("loadOnlineAgents failed", error);
    }
}

function syncRooms() {
    state.rooms = Array.from(state.roomsMap.values());
    state.rooms.sort((a, b) => {
        const aTime = parseDate(a.lastActivity)?.getTime() ?? 0;
        const bTime = parseDate(b.lastActivity)?.getTime() ?? 0;
        return bTime - aTime;
    });
}

function applyFilter() {
    if (!state.searchKeyword) {
        state.filteredRooms = [...state.rooms];
    } else {
        const keyword = state.searchKeyword.toLowerCase();
        state.filteredRooms = state.rooms.filter((room) => {
            return (
                room.roomId.toLowerCase().includes(keyword) ||
                (room.assignedAgent && room.assignedAgent.toLowerCase().includes(keyword)) ||
                (room.lastMessage && room.lastMessage.toLowerCase().includes(keyword))
            );
        });
    }
    renderRoomList();
}

function renderOnlineAgents() {
    if (dom.transferTarget) {
        dom.transferTarget.innerHTML = "";
        const placeholder = document.createElement("option");
        placeholder.value = "";
        placeholder.textContent = "轉接至在線客服";
        dom.transferTarget.appendChild(placeholder);

        state.onlineAgents
            .filter((agent) => agent.id !== state.agentId)
            .forEach((agent) => {
                const option = document.createElement("option");
                option.value = agent.id;
                option.textContent = `${agent.displayName}（${agent.id}）`;
                option.dataset.rooms = (agent.rooms || []).join(", ");
                dom.transferTarget.appendChild(option);
            });

        const hasTargets = dom.transferTarget.options.length > 1;
        dom.transferTarget.disabled = !hasTargets;
        if (dom.transferAgent) {
            dom.transferAgent.disabled = !hasTargets;
        }
    }

    if (dom.onlineAgentList) {
        dom.onlineAgentList.innerHTML = "";
        if (state.onlineAgents.length === 0) {
            const empty = document.createElement("li");
            empty.className = "empty";
            empty.textContent = "尚無在線客服";
            dom.onlineAgentList.appendChild(empty);
        } else {
            state.onlineAgents.forEach((agent) => {
                const item = document.createElement("li");
                item.innerHTML = `<strong>${agent.displayName}</strong><span>${(agent.rooms && agent.rooms.length) ? `房間 ${agent.rooms.join(", ")}` : "待命中"} · ${formatRelative(agent.lastSeen)}</span>`;
                dom.onlineAgentList.appendChild(item);
            });
        }
    }
}

function renderRoomList() {
    const fragment = document.createDocumentFragment();
    const now = Date.now();
    state.filteredRooms.forEach((room) => {
        const card = document.createElement("article");
        card.className = "room-card";
        if (!room.assignedAgent) {
            card.classList.add("waiting");
        } else {
            card.classList.add("assigned");
        }
        if (room.roomId === state.currentRoomId) {
            card.classList.add("active");
        }
        card.dataset.roomId = room.roomId;

        const title = document.createElement("div");
        title.className = "title";
        title.innerHTML = `<span>#${room.roomId}</span>`;
        const badge = document.createElement("span");
        badge.className = "badge";
        const playerOnline = room.connectedPlayerCount ?? room.playerCount ?? 0;
        const agentOnline = room.connectedAgentCount ?? room.agentCount ?? 0;
        badge.textContent = `${room.playerCount} 玩家（${playerOnline} 在線） · ${room.agentCount} 客服（${agentOnline} 在線）`;
        title.appendChild(badge);

        const meta = document.createElement("div");
        meta.className = "meta";
        const lastMessage = room.lastMessage ? room.lastMessage : "尚無訊息";
        meta.textContent = `${lastMessage.slice(0, 32)} · ${formatRelative(room.lastActivity)}`;

        const ping = document.createElement("div");
        ping.className = "ping";
        const dot = document.createElement("span");
        dot.className = "dot";
        const assigned = document.createElement("span");
        assigned.textContent = room.assignedAgent ? `客服 ${room.assignedAgent}` : "未指派";
        ping.append(dot, assigned);

        const metaColumn = document.createElement("div");
        metaColumn.append(meta, ping);

        const timeColumn = document.createElement("div");
        timeColumn.className = "meta";
        const lastActiveTime = parseDate(room.lastActivity);
        const isActive = lastActiveTime && now - lastActiveTime.getTime() <= ACTIVE_THRESHOLD_MS;
        timeColumn.textContent = formatClock(lastActiveTime);
        if (isActive) {
            timeColumn.appendChild(createBadge("活躍", "badge-online"));
        }

        card.append(title, metaColumn, timeColumn);
        card.addEventListener("click", () => selectRoom(room.roomId));
        fragment.appendChild(card);
    });

    dom.roomList.innerHTML = "";
    if (state.filteredRooms.length === 0) {
        const empty = document.createElement("div");
        empty.className = "empty-placeholder";
        empty.textContent = state.rooms.length === 0 ? "目前沒有任何房間" : "找不到符合的房間";
        dom.roomList.appendChild(empty);
    } else {
        dom.roomList.appendChild(fragment);
    }

    dom.roomCounter.textContent = `${state.filteredRooms.length} 個對話`;
}

function createBadge(label, className = "") {
    const badge = document.createElement("span");
    badge.className = `badge ${className}`.trim();
    badge.textContent = label;
    return badge;
}

function buildRoomMeta(summary) {
    if (!summary) return "";
    const playerOnline = summary.connectedPlayerCount ?? summary.playerCount ?? 0;
    const agentOnline = summary.connectedAgentCount ?? summary.agentCount ?? 0;
    const metaParts = [
        `玩家 ${summary.playerCount} 人（在線 ${playerOnline}）`,
        `客服 ${summary.agentCount} 人（在線 ${agentOnline}）`,
    ];
    if (summary.assignedAgent) {
        metaParts.push(`指派客服：${summary.assignedAgent}`);
    }
    return metaParts.join(" ｜ ");
}

async function selectRoom(roomId) {
    if (!roomId || roomId === state.currentRoomId) {
        return;
    }
    if (!state.token) {
        showAuthOverlay();
        return;
    }
    state.currentRoomId = roomId;
    renderRoomList();
    await loadRoomSnapshot(roomId);
    connectSocket(roomId);
}

async function loadRoomSnapshot(roomId) {
    try {
        if (!state.token) return;
        const response = await apiFetch(`/api/rooms/${encodeURIComponent(roomId)}`);
        if (!response.ok) {
            if (response.status === 401) {
                handleUnauthorized();
                return;
            }
            throw new Error(await response.text());
        }
        const snapshot = await response.json();
        renderRoomSnapshot(snapshot);
        updateRoomSummary(snapshot.summary);
    } catch (error) {
        console.error("loadRoomSnapshot failed", error);
    }
}

function renderRoomSnapshot(snapshot) {
    const summary = snapshot.summary;
    dom.roomTitle.textContent = `房號 #${summary.roomId}`;
    dom.roomMeta.textContent = buildRoomMeta(summary);
    state.timelineLastDay = null;
    state.timeline = [];
    dom.messageStream.innerHTML = "";
    if (!snapshot.history || snapshot.history.length === 0) {
        const empty = document.createElement("div");
        empty.className = "empty-placeholder";
        empty.textContent = "尚無任何訊息，請開始與玩家互動。";
        dom.messageStream.appendChild(empty);
    } else {
        snapshot.history.sort((a, b) => (a.sequence || 0) - (b.sequence || 0));
        snapshot.history.forEach((message) => appendMessage(message));
    }
    enableComposer(true);
}

function appendMessage(message, options = {}) {
    const cmd = getCmd(message);
    if (!dom.messageStream) return;

    if (!state.timeline) {
        state.timeline = [];
    }
    state.timeline.push(message);

    const timestamp = parseDate(message.timestamp) || new Date();
    const dayLabel = formatDayLabel(timestamp);
    if (state.timelineLastDay !== dayLabel) {
        const divider = document.createElement("div");
        divider.className = "day-divider";
        divider.textContent = dayLabel;
        dom.messageStream.appendChild(divider);
        state.timelineLastDay = dayLabel;
    }

    if (state.typingBubble && state.typingBubble.isConnected) {
        state.typingBubble.remove();
    }

    const bubble = document.createElement("div");
    bubble.className = "message-bubble fade-in";
    if (cmd === "system.notice") {
        bubble.classList.add("system");
        bubble.textContent = message.content;
    } else if (message.senderId === state.agentId || message.senderRole === "agent") {
        bubble.classList.add("self");
    }

    if (options.highlight) {
        bubble.classList.add("new");
        setTimeout(() => bubble.classList.remove("new"), 900);
    }

    if (cmd !== "system.notice") {
        const header = document.createElement("div");
        header.className = "bubble-header";
        header.innerHTML = `<span>${message.displayName || message.senderRole}</span><span>#${message.sequence || "-"}</span>`;
        const content = document.createElement("div");
        content.className = "content";
        content.textContent = message.content;
        const meta = document.createElement("div");
        meta.className = "message-meta";
        meta.innerHTML = `<span>${formatRelative(timestamp)}</span><span>${formatClock(timestamp)}</span>`;
        bubble.append(header, content, meta);
    }

    dom.messageStream.appendChild(bubble);
    scrollTimelineToBottom();
}

function enableComposer(enabled) {
    dom.messageInput.disabled = !enabled;
    dom.sendButton.disabled = !enabled;
    if (enabled) {
        dom.messageInput.focus();
    }
}

function scrollTimelineToBottom() {
    dom.messageStream.scrollTo({ top: dom.messageStream.scrollHeight, behavior: "smooth" });
}

function connectSocket(roomId) {
    if (!state.agentId) {
        return;
    }
    closeSocket();
    const protocol = window.location.protocol === "https:" ? "wss" : "ws";
    const params = new URLSearchParams({
        roomId,
        role: "agent",
        id: state.agentId,
        name: state.agentDisplayName,
    });
    const socket = new WebSocket(`${protocol}://${window.location.host}/ws?${params.toString()}`);
    state.socket = socket;

    socket.addEventListener("open", () => {
        setConnectionBadge(true);
        if (dom.messageInput.value.trim() !== "") {
            dom.sendButton.disabled = false;
        }
    });

    socket.addEventListener("message", (event) => {
        try {
            const payload = JSON.parse(event.data);
            handleIncoming(payload);
        } catch (error) {
            console.error("invalid message", error);
        }
    });

    socket.addEventListener("close", () => {
        setConnectionBadge(false);
    });

    socket.addEventListener("error", () => {
        setConnectionBadge(false);
    });
}

function closeSocket() {
    if (state.socket) {
        state.socket.close();
        state.socket = null;
    }
}

function setConnectionBadge(connected) {
    const badge = dom.connectionBadge;
    badge.textContent = connected ? "已連線" : "未連線";
    badge.classList.toggle("badge-online", connected);
    badge.classList.toggle("badge-offline", !connected);
}

function handleIncoming(message) {
    const cmd = getCmd(message);
    switch (cmd) {
        case "chat.history": {
            const history = message.history || (message.payload && message.payload.messages) || [];
            dom.messageStream.innerHTML = "";
            state.timeline = [];
            state.timelineLastDay = null;
            history.forEach((item) => appendMessage(item));
            break;
        }
        case "chat.typing": {
            showTypingIndicator(message);
            break;
        }
        case "chat.message": {
            appendMessage(message, { highlight: true });
            updateRoomSummaryFromMessage(message);
            break;
        }
        case "system.notice": {
            appendMessage(message);
            if (message.metadata && message.metadata.assignedAgent) {
                const summary = state.roomsMap.get(message.roomId);
                if (summary) {
                    summary.assignedAgent = message.metadata.assignedAgent;
                    summary.assignedAgentId = message.metadata.assignedAgentId || summary.assignedAgentId;
                    state.roomsMap.set(summary.roomId, summary);
                    syncRooms();
                    applyFilter();
                    updateMetrics();
                }
            }
            break;
        }
        default:
            console.warn("unknown cmd", cmd, message);
    }
}

function showTypingIndicator(message) {
    if (!state.typingBubble) {
        const bubble = document.createElement("div");
        bubble.className = "message-bubble typing";
        bubble.textContent = `${message.displayName || message.senderRole} 正在輸入…`;
        state.typingBubble = bubble;
    }

    if (!state.typingBubble.isConnected) {
        dom.messageStream.appendChild(state.typingBubble);
        scrollTimelineToBottom();
    }

    clearTimeout(state.typingTimer);
    state.typingTimer = setTimeout(() => {
        if (state.typingBubble && state.typingBubble.isConnected) {
            state.typingBubble.remove();
        }
    }, TYPING_TIMEOUT);
}

function updateRoomSummary(summary) {
    state.roomsMap.set(summary.roomId, summary);
    syncRooms();
    applyFilter();
    updateMetrics();
    if (summary.roomId === state.currentRoomId) {
        dom.roomMeta.textContent = buildRoomMeta(summary);
    }
}

function updateRoomSummaryFromMessage(message) {
    const summary = state.roomsMap.get(message.roomId);
    if (!summary) return;
    summary.lastMessage = message.content;
    summary.lastActivity = message.timestamp;
    state.roomsMap.set(summary.roomId, summary);
    syncRooms();
    applyFilter();
    updateMetrics();
    if (message.roomId === state.currentRoomId) {
        dom.roomMeta.textContent = buildRoomMeta(summary);
    }
}

function updateMetrics() {
    const now = Date.now();
    let waiting = 0;
    let active = 0;
    let connectedAggregate = 0;

    state.roomsMap.forEach((room) => {
        if (!room.assignedAgentId) {
            waiting += 1;
        }
        const lastActive = parseDate(room.lastActivity);
        if (lastActive && now - lastActive.getTime() <= ACTIVE_THRESHOLD_MS) {
            active += 1;
        }
        connectedAggregate += room.connectedAgentCount ?? 0;
    });

    const uniqueAgents = state.onlineAgents.length || connectedAggregate;
    dom.waitingCount.textContent = waiting;
    dom.activeCount.textContent = active;
    dom.agentOnlineCount.textContent = uniqueAgents;
    renderDrawerMetrics(waiting, active, uniqueAgents, connectedAggregate);
}

function renderDrawerMetrics(waiting, active, agents, aggregate = agents) {
    if (!dom.drawerMetrics) return;
    dom.drawerMetrics.innerHTML = "";
    const rows = [
        { label: "待指派對話", value: waiting },
        { label: "活躍房間", value: active },
        { label: "在線客服總數", value: agents },
        { label: "房間連線總數", value: aggregate },
    ];

    const topRooms = state.rooms.slice(0, 5);
    topRooms.forEach((room) => {
        rows.push({
            label: `#${room.roomId} · ${room.assignedAgent || "未指派"}`,
            value: formatRelative(room.lastActivity),
        });
    });

    state.onlineAgents.slice(0, 5).forEach((agent) => {
        rows.push({
            label: `客服 ${agent.displayName}`,
            value: (agent.rooms && agent.rooms.length) ? `房間 ${agent.rooms.join(", ")}` : `上次活動 ${formatRelative(agent.lastSeen)}`,
        });
    });

    rows.forEach((row) => {
        const item = document.createElement("div");
        item.className = "metric-row";
        const label = document.createElement("span");
        label.textContent = row.label;
        const value = document.createElement("strong");
        value.textContent = row.value;
        item.append(label, value);
        dom.drawerMetrics.appendChild(item);
    });
}

async function assignRoomTo(agentId, displayName) {
    if (!state.currentRoomId || !agentId) return;
    if (!state.token) {
        showAuthOverlay();
        return;
    }

    const payload = { agentId, displayName };

    try {
        const response = await apiFetch(`/api/rooms/${encodeURIComponent(state.currentRoomId)}/assign`, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(payload),
        });
        if (!response.ok) {
            if (response.status === 401) {
                handleUnauthorized();
                return;
            }
            const errorText = await response.text();
            displayAccountMessage(errorText || "指派失敗", "error");
            return;
        }
        const participant = await response.json();
        const summary = state.roomsMap.get(state.currentRoomId);
        if (summary) {
            summary.assignedAgent = participant.displayName;
            summary.assignedAgentId = participant.id;
            state.roomsMap.set(summary.roomId, summary);
            syncRooms();
            applyFilter();
            updateMetrics();
            dom.roomMeta.textContent = buildRoomMeta(summary);
        }
        displayAccountMessage(`已指派給 ${participant.displayName}`, "success");
        loadOnlineAgents();
    } catch (error) {
        console.error("assign agent failed", error);
        displayAccountMessage("指派失敗，請稍後再試", "error");
    }
}

async function assignCurrentRoom() {
    if (!state.currentRoomId) return;
    if (!state.token) {
        showAuthOverlay();
        return;
    }
    const displayName = dom.agentName.value.trim() || state.agentDisplayName || "客服小幫手";
    state.agentDisplayName = displayName;
    dom.sidebarAgentName.textContent = displayName;
    await assignRoomTo(state.agentId, displayName);
}

function sendMessage(content) {
    if (!state.socket || state.socket.readyState !== WebSocket.OPEN) {
        return;
    }
    const payload = {
        cmd: "chat.message",
        type: "chat.message",
        content,
    };
    state.socket.send(JSON.stringify(payload));
}

function sendTypingSignal() {
    if (!state.socket || state.socket.readyState !== WebSocket.OPEN) {
        return;
    }
    const payload = {
        cmd: "chat.typing",
        type: "chat.typing",
        metadata: { status: "typing" },
    };
    state.socket.send(JSON.stringify(payload));
}

function registerEventListeners() {
    dom.refreshRooms.addEventListener("click", () => {
        loadRooms();
        loadOnlineAgents();
    });
    dom.roomSearch.addEventListener("input", (event) => {
        state.searchKeyword = event.target.value.trim();
        applyFilter();
    });
    dom.agentName.addEventListener("input", (event) => {
        state.agentDisplayName = event.target.value.trim() || "客服小幫手";
        dom.sidebarAgentName.textContent = state.agentDisplayName;
        updateAvatar(state.agentDisplayName);
    });
    dom.assignAgent.addEventListener("click", assignCurrentRoom);
    if (dom.transferAgent) {
        dom.transferAgent.addEventListener("click", () => {
            const targetId = dom.transferTarget.value;
            if (!targetId) return;
            const target = state.onlineAgents.find((agent) => agent.id === targetId);
            const displayName = target ? target.displayName : targetId;
            assignRoomTo(targetId, displayName);
        });
    }
    if (dom.transferTarget) {
        dom.transferTarget.addEventListener("change", () => {
            if (dom.transferAgent) {
                dom.transferAgent.disabled = dom.transferTarget.value === "";
            }
        });
    }
    dom.openMetrics.addEventListener("click", () => {
        dom.metricsDrawer.hidden = false;
    });
    dom.closeMetrics.addEventListener("click", () => {
        dom.metricsDrawer.hidden = true;
    });
    dom.metricsDrawer.addEventListener("click", (event) => {
        if (event.target === dom.metricsDrawer) {
            dom.metricsDrawer.hidden = true;
        }
    });

    dom.toggleSidebar.addEventListener("click", () => {
        state.isSidebarCollapsed = !state.isSidebarCollapsed;
        dom.workspace.classList.toggle("collapsed", state.isSidebarCollapsed);
        dom.workspaceList.classList.toggle("collapsed", state.isSidebarCollapsed);
        dom.toggleSidebar.textContent = state.isSidebarCollapsed ? "展開" : "收合";
        if (dom.openSidebarHandle) {
            dom.openSidebarHandle.hidden = !state.isSidebarCollapsed;
        }
    });

    if (dom.openSidebarHandle) {
        dom.openSidebarHandle.addEventListener("click", () => {
            state.isSidebarCollapsed = false;
            dom.workspace.classList.remove("collapsed");
            dom.workspaceList.classList.remove("collapsed");
            dom.toggleSidebar.textContent = "收合";
            dom.openSidebarHandle.hidden = true;
        });
    }

    dom.messageForm.addEventListener("submit", (event) => {
        event.preventDefault();
        const content = dom.messageInput.value.trim();
        if (!content) return;
        sendMessage(content);
        dom.messageInput.value = "";
        autoGrowTextarea();
    });

    dom.messageInput.addEventListener("input", () => {
        dom.sendButton.disabled = dom.messageInput.value.trim() === "";
        autoGrowTextarea();
        sendTypingSignal();
    });

    dom.messageInput.addEventListener("keydown", (event) => {
        if (event.key === "Enter" && !event.shiftKey) {
            event.preventDefault();
            const content = dom.messageInput.value.trim();
            if (content) {
                sendMessage(content);
                dom.messageInput.value = "";
                autoGrowTextarea();
            }
        }
    });

    dom.sendTyping.addEventListener("click", () => sendTypingSignal());

    window.addEventListener("beforeunload", () => closeSocket());

    if (dom.loginForm) {
        dom.loginForm.addEventListener("submit", handleLoginSubmit);
    }
    if (dom.logoutButton) {
        dom.logoutButton.addEventListener("click", () => logout());
    }
    if (dom.openAccountDrawer) {
        dom.openAccountDrawer.addEventListener("click", () => {
            if (!state.token) {
                showAuthOverlay();
                return;
            }
            updateAccountControls();
            renderOnlineAgents();
            dom.accountDrawer.hidden = false;
            loadAgencySettingsList({ silent: true });
        });
    }
    if (dom.closeAccountDrawer) {
        dom.closeAccountDrawer.addEventListener("click", () => {
            dom.accountDrawer.hidden = true;
        });
    }
    if (dom.accountDrawer) {
        dom.accountDrawer.addEventListener("click", (event) => {
            if (event.target === dom.accountDrawer) {
                dom.accountDrawer.hidden = true;
            }
        });
    }
    if (dom.createAgentForm) {
        dom.createAgentForm.addEventListener("submit", async (event) => {
            event.preventDefault();
            if (!state.account || state.account.username !== "admin01") {
                displayAccountMessage("僅 admin01 可以建立客服帳號", "error");
                return;
            }
            const username = dom.agentUsername.value.trim().toLowerCase();
            const password = dom.agentPassword.value;
            const displayName = dom.agentDisplay.value.trim();
            const agency = dom.agentAgency ? dom.agentAgency.value.trim() : "";
            if (!username || !password) {
                displayAccountMessage("請填寫客服帳號與密碼", "error");
                return;
            }
            const account = await registerAccount("admin", username, password, displayName, agency);
            if (account) {
                dom.agentUsername.value = "";
                dom.agentPassword.value = "";
                dom.agentDisplay.value = "";
                if (dom.agentAgency) dom.agentAgency.value = "";
                displayAccountMessage(`已建立客服帳號 ${account.username}`, "success");
                loadOnlineAgents();
            }
        });
    }
    if (dom.createPlayerForm) {
        dom.createPlayerForm.addEventListener("submit", async (event) => {
            event.preventDefault();
            const username = dom.playerUsername.value.trim().toLowerCase();
            const password = dom.playerPassword.value;
            const displayName = dom.playerDisplay.value.trim();
            const agency = dom.playerAgency ? dom.playerAgency.value.trim() : "";
            if (!username || !password) {
                displayAccountMessage("請填寫玩家帳號與密碼", "error");
                return;
            }
            const account = await registerAccount("player", username, password, displayName, agency);
            if (account) {
                dom.playerUsername.value = "";
                dom.playerPassword.value = "";
                dom.playerDisplay.value = "";
                if (dom.playerAgency) dom.playerAgency.value = "";
                displayAccountMessage(`已建立玩家帳號 ${account.username}`, "success");
            }
        });
    }
    if (dom.agencySettingsForm) {
        dom.agencySettingsForm.addEventListener("submit", saveAgencySettings);
    }
    if (dom.settingsSelect) {
        dom.settingsSelect.addEventListener("change", () => {
            const agency = dom.settingsSelect.value;
            if (agency) {
                fillAgencySettingsForm(agency);
            } else {
                resetAgencySettingsForm();
                displayAgencySettingsMessage("可輸入代理代碼後儲存設定", "info");
            }
        });
    }
    if (dom.resetAgencySettings) {
        dom.resetAgencySettings.addEventListener("click", () => {
            resetAgencySettingsForm();
            displayAgencySettingsMessage("欄位已清除", "info");
        });
    }
}

function autoGrowTextarea() {
    dom.messageInput.style.height = "auto";
    dom.messageInput.style.height = `${dom.messageInput.scrollHeight}px`;
}

function initialize() {
    dom.sidebarAgentName.textContent = state.agentDisplayName;
    dom.agentName.disabled = true;
    dom.assignAgent.disabled = true;
    updateAvatar(state.agentDisplayName);
    registerEventListeners();
    renderRoomList();
    renderOnlineAgents();
    resetRoomView();
    restoreSession();
}

initialize();
