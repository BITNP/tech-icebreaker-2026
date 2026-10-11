// 与后端 /api 的交互。
//
// token 只存内存：刷新页面后由 Keycloak 的 SSO cookie 静默换新，
// 所以这里没有 localStorage、没有 refresh token。
const REDIRECT_FLAG = 'icebreaker.redirecting'

let token = ''
let redirecting = false

// captureToken 从 /?token=... 里取回登录结果，并把地址还原干净。
// 返回 true 表示这次是刚登录回来。
export function captureToken() {
  const url = new URL(window.location.href)
  const t = url.searchParams.get('token') || ''
  if (!t) return false
  token = t
  url.searchParams.delete('token')
  window.history.replaceState(null, '', url.pathname + url.search + url.hash)
  sessionStorage.removeItem(REDIRECT_FLAG)
  return true
}

export function hasToken() {
  return token !== ''
}

// login 走一次 Keycloak 授权码登录。Keycloak 那边 SSO 还在时用户无感。
// next 是登录后要回到的本站路径（签到页与管理台各回各的）。
export function login(next) {
  if (redirecting) return
  redirecting = true
  window.location.href = next
    ? '/auth/login?next=' + encodeURIComponent(next)
    : '/auth/login'
}

// autoLoginOnce 只自动跳一次：防止"没权限→跳登录→回来还是没 token"来回弹。
export function autoLoginOnce(next) {
  if (sessionStorage.getItem(REDIRECT_FLAG)) return false
  sessionStorage.setItem(REDIRECT_FLAG, '1')
  login(next)
  return true
}

// currentRoute 当前 hash 路由，用于令牌失效后回到原处（大屏/管理台不该被踢回首页）。
function currentRoute() {
  const hash = window.location.hash.replace(/^#/, '')
  return '/#' + (hash || '/')
}

// auth=true 表示带上网关令牌；401 时静默重走一次 SSO。
async function request(path, { method = 'GET', body, auth = false } = {}) {
  const headers = {}
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  if (auth) headers.Authorization = 'Bearer ' + token

  let res
  try {
    res = await fetch(path, {
      method,
      headers,
      body: body === undefined ? undefined : JSON.stringify(body),
    })
  } catch (e) {
    const err = new Error('网络不通，请检查连接')
    err.code = 'network'
    throw err
  }

  const text = await res.text()
  let data = null
  try {
    data = text ? JSON.parse(text) : null
  } catch (e) {
    data = null
  }

  if (!res.ok) {
    const err = new Error(describe(res.status, data))
    err.status = res.status
    err.code = (data && data.error) || 'http_' + res.status
    err.data = data
    // 管理接口 401 = token 过期：重走一次 SSO，不打扰用户。
    if (auth && res.status === 401) autoLoginOnce(currentRoute())
    throw err
  }
  return data
}

const MESSAGES = {
  signup_closed: '签到还没开始（或已经结束），等主持人说开始',
  bad_name: '名字不能为空',
  name_too_long: '名字太长了（最多 20 个字）',
  pool_empty: '候选池里还没有人，先让大家签到',
  not_enough_participants: '候选池人数不够，减少抽取人数或先加人',
  no_current_round: '当前没有进行中的轮次',
  not_admin: '这个 Keycloak 账号不在管理员白名单里',
  no_permission: '这个组织账号没有这项权限',
  bad_role: '角色名不对',
  config_admin_immutable: '配置文件里的根管理员改不了',
  last_admin: '不能撤掉最后一个管理员',
  unauthorized: '登录状态已失效，正在重新登录…',
  bad_count: '抽取人数要大于 0',
  bad_scope: '重置范围不对',
  bad_mode: '导入方式不对',
  bad_id: '不认识的参与者编号',
  not_found: '没找到',
  internal_error: '服务端出错了，看后台日志',
}

function describe(status, data) {
  const code = data && data.error
  if (code && MESSAGES[code]) {
    // 人数不够时把剩余人数说清楚，现场好判断
    if (code === 'not_enough_participants' && data.available !== undefined) {
      return MESSAGES[code] + '（当前可抽 ' + data.available + ' 人）'
    }
    return MESSAGES[code]
  }
  return '请求失败（HTTP ' + status + '）'
}

export const api = {
  // 公开侧
  screenState: () => request('/api/v1/screen/state', { auth: true }),
  screenRound: () => request('/api/v1/screen/round', { auth: true }),
  qr: (data) => '/api/v1/qr?data=' + encodeURIComponent(data),

  // 登录就能用
  me: () => request('/api/v1/me', { auth: true }),
  checkin: () => request('/api/v1/checkin', { method: 'POST', auth: true }),

  // 部长介绍：列表公开，写要 minister 及以上
  directors: () => request('/api/v1/directors', { auth: true }),
  saveDirector: (payload) =>
    request('/api/v1/directors', { method: 'POST', body: payload, auth: true }),
  deleteMyDirector: () => request('/api/v1/directors/me', { method: 'DELETE', auth: true }),
  deleteDirector: (sub) =>
    request('/api/v1/directors/' + encodeURIComponent(sub), { method: 'DELETE', auth: true }),

  // 操作员侧：抽奖、大屏、名单
  adminState: () => request('/api/v1/admin/state', { auth: true }),
  openSignup: () => request('/api/v1/admin/checkin/open', { method: 'POST', auth: true }),
  closeSignup: () => request('/api/v1/admin/checkin/close', { method: 'POST', auth: true }),
  addParticipant: (name) =>
    request('/api/v1/admin/participants', { method: 'POST', body: { name }, auth: true }),
  deleteParticipant: (id) =>
    request('/api/v1/admin/participants/' + id, { method: 'DELETE', auth: true }),
  importNames: (names, mode) =>
    request('/api/v1/admin/import', { method: 'POST', body: { names, mode }, auth: true }),
  draw: (payload) => request('/api/v1/admin/rounds', { method: 'POST', body: payload, auth: true }),
  cursor: (revealedIndex) =>
    request('/api/v1/admin/rounds/current/cursor', {
      method: 'POST',
      body: { revealedIndex },
      auth: true,
    }),
  skip: () => request('/api/v1/admin/rounds/current/skip', { method: 'POST', auth: true }),
  endRound: () => request('/api/v1/admin/rounds/current/end', { method: 'POST', auth: true }),
  reset: (scope) => request('/api/v1/admin/reset', { method: 'POST', body: { scope }, auth: true }),

  // 角色管理（只有 admin）
  roles: () => request('/api/v1/admin/roles', { auth: true }),
  setRole: (sub, role, name) =>
    request('/api/v1/admin/roles', { method: 'POST', body: { sub, role, name }, auth: true }),
  deleteRole: (sub) =>
    request('/api/v1/admin/roles/' + encodeURIComponent(sub), { method: 'DELETE', auth: true }),
}

// downloadExport 带 Authorization 取文件再下载：导出接口不能当普通链接点。
export async function downloadExport(what) {
  const res = await fetch('/api/v1/admin/export/' + what, {
    headers: { Authorization: 'Bearer ' + token },
  })
  if (!res.ok) {
    if (res.status === 401) login()
    throw new Error('导出失败（HTTP ' + res.status + '）')
  }
  const blob = await res.blob()
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = what === 'results' ? 'results.txt' : 'participants.txt'
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
}
