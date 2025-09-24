const state = {
    roomId: null,
    playerId: null,
    displayName: "玩家",
    token: null,
    account: null,
    assignedAgent: null,
    socket: null,
    typingTimer: null,
    typingBubble: null,
    suppressDisconnectNotice: false,
    timeline: [],
    timelineLastDay: null,
    connected: false,
};

const dom = {
    loginForm: document.getElementById("playerLoginForm"),
    loginUsername: document.getElementById("loginUsername"),
    loginPassword: document.getElementById("loginPassword"),
    loginError: document.getElementById("playerLoginError"),
    registerPanel: document.getElementById("registerPanel"),
    loginPanel: document.getElementById("loginPanel"),
    registerForm: document.getElementById("playerRegisterForm"),
    registerUsername: document.getElementById("registerUsername"),
    registerPassword: document.getElementById("registerPassword"),
    registerDisplay: document.getElementById("registerDisplay"),
    registerAgency: document.getElementById("registerAgency"),
    registerError: document.getElementById("playerRegisterError"),
    showRegister: document.getElementById("showRegister"),
    showLogin: document.getElementById("showLogin"),
    accountPanel: document.getElementById("accountPanel"),
    accountSummary: document.getElementById("accountSummary"),
    playerLogout: document.getElementById("playerLogout"),
    playerName: document.getElementById("playerName"),
    startChat: document.getElementById("startChat"),
    roomIndicator: document.getElementById("roomIndicator"),
    connectionState: document.getElementById("connectionState"),
    connectionDot: document.getElementById("connectionDot"),
    chatTitle: document.getElementById("chatTitle"),
    chatSubtitle: document.getElementById("chatSubtitle"),
    assignedAgentIndicator: document.getElementById("assignedAgentIndicator"),
    messageTimeline: document.getElementById("messageTimeline"),
    chatForm: document.getElementById("chatForm"),
    messageInput: document.getElementById("messageInput"),
    sendButton: document.getElementById("sendButton"),
    typingSignal: document.getElementById("typingSignal"),
    requestHistory: document.getElementById("requestHistory"),
    leaveRoom: document.getElementById("leaveRoom"),
};

const TYPING_TIMEOUT = 1200;
const PLAYER_TOKEN_KEY = "imPlayerToken";
const DEFAULT_CHAT_TITLE = "客服連線準備中";
const DEFAULT_CHAT_SUBTITLE = "點擊左側開始對話，系統將建立房間並等待客服加入。";
const WAITING_ASSIGNMENT_SUBTITLE = "客服正在安排中，您可先留言";

function apiFetch(url, options = {}) {
    const init = { ...options };
    const headers = new Headers(options.headers || {});
    if (state.token) {
        headers.set("Authorization", `Bearer ${state.token}`);
    }
    init.headers = headers;
    return fetch(url, init);
}

function persistToken(token) {
    if (token) {
        localStorage.setItem(PLAYER_TOKEN_KEY, token);
    } else {
        localStorage.removeItem(PLAYER_TOKEN_KEY);
    }
}

function showLoginPanel() {
    if (dom.loginPanel) dom.loginPanel.hidden = false;
    if (dom.registerPanel) dom.registerPanel.hidden = true;
    if (dom.accountPanel) dom.accountPanel.hidden = true;
}

function showRegisterPanel() {
    if (dom.loginPanel) dom.loginPanel.hidden = true;
    if (dom.registerPanel) dom.registerPanel.hidden = false;
    if (dom.accountPanel) dom.accountPanel.hidden = true;
}

function showAccountPanel() {
    if (dom.loginPanel) dom.loginPanel.hidden = true;
    if (dom.registerPanel) dom.registerPanel.hidden = true;
    if (dom.accountPanel) dom.accountPanel.hidden = false;
}

function updateAccountSummary() {
    if (!dom.accountSummary) return;
    if (state.account) {
        const roleText = state.account.role === "admin" ? "客服" : "玩家";
        const agency = state.account.agency || "default";
        dom.accountSummary.textContent = `帳號：${state.account.username} · 代理：${agency} · 身分：${roleText}`;
    } else {
        dom.accountSummary.textContent = "尚未登入";
    }
}

function applySession(account, token) {
    state.account = account;
    state.token = token;
    state.playerId = account.username;
    state.displayName = account.displayName || account.username;
    dom.playerName.value = state.displayName;
    dom.playerName.disabled = false;
    dom.startChat.disabled = false;
    dom.requestHistory.disabled = true;
    persistToken(token);
    updateAccountSummary();
    showAccountPanel();
    dom.roomIndicator.textContent = "--";
    dom.chatTitle.textContent = DEFAULT_CHAT_TITLE;
    dom.chatSubtitle.textContent = DEFAULT_CHAT_SUBTITLE;
    resetTimeline();
}

function clearSession() {
    closeSocket({ silent: true });
    state.account = null;
    state.token = null;
    state.playerId = null;
    state.displayName = "玩家";
    state.assignedAgent = null;
    state.connected = false;
    state.roomId = null;
    state.timeline = [];
    state.timelineLastDay = null;
    if (state.typingBubble && state.typingBubble.isConnected) {
        state.typingBubble.remove();
    }
    state.typingBubble = null;
    clearTimeout(state.typingTimer);
    state.typingTimer = null;
    persistToken(null);
    dom.playerName.value = "";
    dom.playerName.disabled = true;
    dom.startChat.disabled = true;
    dom.requestHistory.disabled = true;
    if (dom.messageInput) {
        dom.messageInput.value = "";
        autoGrowTextarea();
    }
    updateConnection(false);
    setComposerEnabled(false);
    resetTimeline();
    dom.roomIndicator.textContent = "--";
    dom.chatTitle.textContent = DEFAULT_CHAT_TITLE;
    dom.chatSubtitle.textContent = DEFAULT_CHAT_SUBTITLE;
    const url = new URL(window.location.href);
    if (url.searchParams.has("room")) {
        url.searchParams.delete("room");
        window.history.replaceState({}, "", url.toString());
    }
    updateAccountSummary();
    showLoginPanel();
}

function setAssignedAgent(name) {
    state.assignedAgent = name || null;
    if (dom.assignedAgentIndicator) {
        dom.assignedAgentIndicator.textContent = name ? name : "尚未指派";
    }
    if (name) {
        dom.chatSubtitle.textContent = `客服 ${name} 正在為您服務`;
    } else if (state.connected) {
        dom.chatSubtitle.textContent = WAITING_ASSIGNMENT_SUBTITLE;
    } else {
        dom.chatSubtitle.textContent = DEFAULT_CHAT_SUBTITLE;
    }
}

async function loginRequest(username, password) {
    try {
        const response = await fetch("/api/auth/login", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ username, password }),
        });
        if (!response.ok) {
            const text = await response.text();
            return { error: text || "登入失敗" };
        }
        const payload = await response.json();
        return { data: payload };
    } catch (error) {
        console.error("login request failed", error);
        return { error: "登入失敗，請稍後再試" };
    }
}

async function handleLoginSubmit(event) {
    event.preventDefault();
    const username = dom.loginUsername.value.trim().toLowerCase();
    const password = dom.loginPassword.value;
    if (!username || !password) {
        if (dom.loginError) dom.loginError.textContent = "請輸入帳號與密碼";
        return;
    }
    const result = await loginRequest(username, password);
    if (result.error) {
        if (dom.loginError) dom.loginError.textContent = result.error;
        return;
    }
    if (dom.loginError) dom.loginError.textContent = "";
    dom.loginUsername.value = "";
    dom.loginPassword.value = "";
    await onPlayerLogin(result.data.account, result.data.token);
}

async function handleRegisterSubmit(event) {
    event.preventDefault();
    const username = dom.registerUsername.value.trim().toLowerCase();
    const password = dom.registerPassword.value;
    const displayName = dom.registerDisplay.value.trim();
    const agency = dom.registerAgency ? dom.registerAgency.value.trim() : "";
    if (!username || !password) {
        if (dom.registerError) dom.registerError.textContent = "請填寫帳號與密碼";
        return;
    }
    try {
        const response = await fetch("/api/auth/register", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ username, password, displayName, role: "player", agency }),
        });
        if (!response.ok) {
            const text = await response.text();
            if (dom.registerError) dom.registerError.textContent = text || "註冊失敗";
            return;
        }
        if (dom.registerError) dom.registerError.textContent = "";
        dom.registerUsername.value = "";
        dom.registerPassword.value = "";
        dom.registerDisplay.value = "";
        if (dom.registerAgency) dom.registerAgency.value = "";
        const loginResult = await loginRequest(username, password);
        if (loginResult.error) {
            if (dom.registerError) dom.registerError.textContent = loginResult.error;
            return;
        }
        await onPlayerLogin(loginResult.data.account, loginResult.data.token);
    } catch (error) {
        console.error("register failed", error);
        if (dom.registerError) dom.registerError.textContent = "註冊失敗，請稍後再試";
    }
}

async function logout() {
    if (state.token) {
        try {
            await apiFetch("/api/auth/logout", { method: "POST" });
        } catch (error) {
            console.warn("logout failed", error);
        }
    }
    clearSession();
}

async function restoreSession() {
    const stored = localStorage.getItem(PLAYER_TOKEN_KEY);
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
        await onPlayerLogin(account, stored);
    } catch (error) {
        console.error("restore session failed", error);
        clearSession();
    }
}

async function onPlayerLogin(account, token) {
    applySession(account, token);
    setAssignedAgent(null);
}

function resetTimeline(placeholder = "尚未建立對話") {
    state.timeline = [];
    state.timelineLastDay = null;
    if (dom.messageTimeline) {
        dom.messageTimeline.innerHTML = `<div class="wk-placeholder">${placeholder}</div>`;
    }
}

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

function formatDayLabel(value) {
    const date = typeof value === "string" ? parseDate(value) : value;
    if (!date) return "";
    const base = new Date();
    base.setHours(0, 0, 0, 0);
    const compare = new Date(date);
    compare.setHours(0, 0, 0, 0);
    const diff = Math.round((base - compare) / (24 * 60 * 60 * 1000));
    if (diff === 0) return "今天";
    if (diff === 1) return "昨天";
    return compare.toLocaleDateString("zh-TW", { month: "2-digit", day: "2-digit", weekday: "short" });
}

function getCmd(message) {
    return message.cmd || message.type || "";
}

function ensureRoomId() {
    const url = new URL(window.location.href);
    let room = url.searchParams.get("room");
    if (!room) {
        room = `room-${Date.now()}`;
        url.searchParams.set("room", room);
        window.history.replaceState({}, "", url.toString());
    }
    return room;
}

function connect() {
    if (!state.account || !state.token) {
        if (dom.loginError) {
            dom.loginError.textContent = "請先登入後再開始對話";
        }
        showLoginPanel();
        return;
    }

    state.displayName = dom.playerName.value.trim() || state.displayName || state.account.displayName || state.account.username;
    state.playerId = state.account.username;
    state.roomId = ensureRoomId();
    dom.roomIndicator.textContent = state.roomId;
    dom.chatTitle.textContent = `與客服的對話 #${state.roomId}`;
    dom.chatSubtitle.textContent = DEFAULT_CHAT_SUBTITLE;
    setAssignedAgent(null);

    closeSocket({ silent: true });

    const protocol = window.location.protocol === "https:" ? "wss" : "ws";
    const params = new URLSearchParams({
        roomId: state.roomId,
        role: "player",
        id: state.playerId,
        name: state.displayName,
    });
    const socket = new WebSocket(`${protocol}://${window.location.host}/ws?${params.toString()}`);
    state.socket = socket;

    socket.addEventListener("open", () => {
        if (dom.messageInput) {
            dom.messageInput.value = "";
            autoGrowTextarea();
        }
        updateConnection(true);
        setComposerEnabled(true);
        appendSystem("已連線，客服稍候即將加入");
        requestHistorySync();
    });

    socket.addEventListener("message", (event) => {
        try {
            const message = JSON.parse(event.data);
            handleIncoming(message);
        } catch (error) {
            console.error("invalid message", error);
        }
    });

    socket.addEventListener("close", () => {
        updateConnection(false);
        setComposerEnabled(false);
        if (state.suppressDisconnectNotice) {
            state.suppressDisconnectNotice = false;
            return;
        }
        appendSystem("連線已關閉，您可重新點擊開始對話");
    });

    socket.addEventListener("error", () => {
        updateConnection(false);
        setComposerEnabled(false);
        appendSystem("連線發生異常，請稍後再試");
    });
}

function closeSocket({ silent = false } = {}) {
    if (state.socket) {
        if (silent) {
            state.suppressDisconnectNotice = true;
        }
        state.socket.close();
        state.socket = null;
    }
}

function updateConnection(connected) {
    state.connected = connected;
    dom.connectionState.textContent = connected ? "已連線" : "未連線";
    dom.connectionDot.classList.toggle("online", connected);
    if (dom.requestHistory) {
        dom.requestHistory.disabled = !connected;
    }
    if (dom.typingSignal) {
        dom.typingSignal.disabled = !connected;
    }
    if (!connected) {
        setAssignedAgent(null);
    } else {
        setAssignedAgent(state.assignedAgent);
    }
}

function setComposerEnabled(enabled) {
    dom.messageInput.disabled = !enabled;
    dom.sendButton.disabled = !enabled || dom.messageInput.value.trim() === "";
    if (enabled) {
        dom.messageInput.focus();
    }
}

function handleIncoming(message) {
    const cmd = getCmd(message);
    switch (cmd) {
        case "chat.history":
            renderHistory(message.history || (message.payload && message.payload.messages) || []);
            break;
        case "chat.typing":
            showTyping(message);
            break;
        case "chat.message":
            appendMessage(message);
            break;
        case "system.notice":
            if (message.metadata && message.metadata.assignedAgent) {
                setAssignedAgent(message.metadata.assignedAgent);
            } else {
                appendSystem(message.content);
            }
            break;
        default:
            console.warn("unknown message", message);
    }
}

function renderHistory(history) {
    if (!Array.isArray(history) || history.length === 0) {
        resetTimeline("尚未有訊息，您可以先行留言");
        return;
    }
    state.timeline = [];
    state.timelineLastDay = null;
    if (dom.messageTimeline) {
        dom.messageTimeline.innerHTML = "";
    }
    history.sort((a, b) => (a.sequence || 0) - (b.sequence || 0));
    history.forEach((item) => {
        if (getCmd(item) === "system.notice" && item.metadata && item.metadata.assignedAgent) {
            setAssignedAgent(item.metadata.assignedAgent);
            return;
        }
        appendMessage(item);
    });
}

function appendMessage(message) {
    const cmd = getCmd(message);
    const timestamp = parseDate(message.timestamp) || new Date();

    if (!state.timeline) state.timeline = [];
    state.timeline.push(message);

    if (cmd === "system.notice" && message.metadata && message.metadata.assignedAgent) {
        setAssignedAgent(message.metadata.assignedAgent);
        return;
    }

    if (dom.messageTimeline && dom.messageTimeline.firstElementChild && dom.messageTimeline.firstElementChild.classList.contains("wk-placeholder")) {
        dom.messageTimeline.innerHTML = "";
    }

    if (!dom.messageTimeline) {
        return;
    }

    const dayLabel = formatDayLabel(timestamp);
    if (state.timelineLastDay !== dayLabel) {
        const divider = document.createElement("div");
        divider.className = "wk-divider";
        divider.textContent = dayLabel;
        dom.messageTimeline.appendChild(divider);
        state.timelineLastDay = dayLabel;
    }

    if (state.typingBubble && state.typingBubble.isConnected) {
        state.typingBubble.remove();
    }

    const bubble = document.createElement("div");
    bubble.className = "wk-message";
    if (cmd === "system.notice") {
        bubble.classList.add("system");
        bubble.textContent = message.content;
        dom.messageTimeline.appendChild(bubble);
        scrollToBottom();
        return;
    }

    if (message.senderId === state.playerId || message.senderRole === "player") {
        bubble.classList.add("self");
    }

    const header = document.createElement("div");
    header.className = "wk-header";
    header.innerHTML = `<span>${message.displayName || message.senderRole}</span><span>#${message.sequence || "-"}</span>`;

    const content = document.createElement("div");
    content.className = "wk-content";
    content.textContent = message.content;

    const meta = document.createElement("div");
    meta.className = "wk-meta";
    meta.innerHTML = `<span>${formatRelative(timestamp)}</span><span>${formatClock(timestamp)}</span>`;

    bubble.append(header, content, meta);
    dom.messageTimeline.appendChild(bubble);
    scrollToBottom();
}

function appendSystem(text) {
    if (!dom.messageTimeline) return;
    if (dom.messageTimeline.firstElementChild && dom.messageTimeline.firstElementChild.classList.contains("wk-placeholder")) {
        dom.messageTimeline.innerHTML = "";
    }
    const bubble = document.createElement("div");
    bubble.className = "wk-message system";
    bubble.textContent = text;
    dom.messageTimeline.appendChild(bubble);
    scrollToBottom();
}

function scrollToBottom() {
    dom.messageTimeline.scrollTo({ top: dom.messageTimeline.scrollHeight, behavior: "smooth" });
}

function showTyping(message) {
    if (!state.typingBubble) {
        const bubble = document.createElement("div");
        bubble.className = "wk-typing";
        bubble.textContent = `${message.displayName || message.senderRole} 正在輸入…`;
        state.typingBubble = bubble;
    } else {
        state.typingBubble.textContent = `${message.displayName || message.senderRole} 正在輸入…`;
    }

    if (!state.typingBubble.isConnected) {
        dom.messageTimeline.appendChild(state.typingBubble);
        scrollToBottom();
    }

    clearTimeout(state.typingTimer);
    state.typingTimer = setTimeout(() => {
        if (state.typingBubble && state.typingBubble.isConnected) {
            state.typingBubble.remove();
        }
    }, TYPING_TIMEOUT);
}

function sendMessage(content) {
    if (!state.socket || state.socket.readyState !== WebSocket.OPEN) {
        return;
    }
    state.socket.send(
        JSON.stringify({
            cmd: "chat.message",
            type: "chat.message",
            content,
        })
    );
}

function sendTyping() {
    if (!state.socket || state.socket.readyState !== WebSocket.OPEN) {
        return;
    }
    state.socket.send(
        JSON.stringify({
            cmd: "chat.typing",
            type: "chat.typing",
            metadata: { status: "typing" },
        })
    );
}

function requestHistorySync() {
    if (!state.socket || state.socket.readyState !== WebSocket.OPEN) {
        return;
    }
    state.socket.send(
        JSON.stringify({
            cmd: "chat.history",
            type: "chat.history",
        })
    );
}

function leaveRoom() {
    closeSocket({ silent: true });
    updateConnection(false);
    setComposerEnabled(false);
    setAssignedAgent(null);
    resetTimeline();
    appendSystem("您已離開聊天室，可再次點擊開始對話建立新連線。");
}

function autoGrowTextarea() {
    dom.messageInput.style.height = "auto";
    dom.messageInput.style.height = `${dom.messageInput.scrollHeight}px`;
}

function registerEvents() {
    dom.startChat.addEventListener("click", connect);
    dom.requestHistory.addEventListener("click", requestHistorySync);
    dom.leaveRoom.addEventListener("click", leaveRoom);

    if (dom.playerName) {
        dom.playerName.addEventListener("input", () => {
            state.displayName = dom.playerName.value.trim();
        });
    }

    if (dom.loginForm) {
        dom.loginForm.addEventListener("submit", handleLoginSubmit);
    }
    if (dom.registerForm) {
        dom.registerForm.addEventListener("submit", handleRegisterSubmit);
    }
    if (dom.showRegister) {
        dom.showRegister.addEventListener("click", () => {
            if (dom.loginError) dom.loginError.textContent = "";
            if (dom.registerError) dom.registerError.textContent = "";
            showRegisterPanel();
        });
    }
    if (dom.showLogin) {
        dom.showLogin.addEventListener("click", () => {
            if (dom.loginError) dom.loginError.textContent = "";
            if (dom.registerError) dom.registerError.textContent = "";
            showLoginPanel();
        });
    }
    if (dom.playerLogout) {
        dom.playerLogout.addEventListener("click", () => logout());
    }

    dom.chatForm.addEventListener("submit", (event) => {
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
        sendTyping();
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

    dom.typingSignal.addEventListener("click", sendTyping);

    window.addEventListener("beforeunload", () => closeSocket({ silent: true }));
}

function initialize() {
    dom.playerName.value = "";
    dom.startChat.disabled = true;
    dom.requestHistory.disabled = true;
    updateAccountSummary();
    showLoginPanel();
    resetTimeline();
    registerEvents();
    restoreSession();
}

initialize();
