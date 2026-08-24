// 与后端 domain 模型对应的 TypeScript 类型

export interface Result<T = unknown> {
  code: number
  message: string
  data: T
  t: number
}

export interface Login {
  username: string
  password: string
  ip?: string
  key?: string
}

export interface NotificationConfig {
  enable: boolean
  retry: number
  comment: string
  notificationTemplate: string
  notificationType: string
  mailSMTPHost: string
  mailSMTPPort: number
  mailFrom: string
  mailPassword: string
  mailSSLEnable: boolean
  mailTLSEnable: boolean
  mailAddressee: string
  mailImage: boolean
  serverChanType: string
  serverChanSendKey: string
  serverChan3ApiUrl: string
  telegramBotToken: string
  telegramChatId: string
  telegramTopicId: number
  telegramApiHost: string
  telegramImage: boolean
  telegramFormat: string
  webHookMethod: string
  webHookUrl: string
  webHookHeader: string
  webHookBody: string
  embyHost: string
  embyApiKey: string
  embyRefreshViewIds: string
  shell: string
  barkServerUrl: string
  barkDeviceKeys: string
  barkGroup: string
  barkUseMarkdown: boolean
  barkLevel: string
  barkVolume: number
  statusList: string[]
  sort: number
}

export interface Config {
  mikanHost: string
  tmdbApi: string
  tmdbApiKey: string
  tmdbImage: string
  tmdbAnime: boolean
  downloadToolType: string
  downloadRetry: number
  pan115Cookie: string
  downloadPathTemplate: string
  ovaDownloadPathTemplate: string
  delayedDownload: number
  rssSleepMinutes: number
  rename: boolean
  rss: boolean
  rssTimeout: number
  fileExist: boolean
  offset: boolean
  titleYear: boolean
  autoDisabled: boolean
  skip5: boolean
  standbyRss: boolean
  coexist: boolean
  logsMax: number
  debug: boolean
  procrastinatingMasterOnly: boolean
  proxy: boolean
  proxyHost: string
  proxyPort: number
  proxyUsername: string
  proxyPassword: string
  login: Login
  multiLoginForbidden: boolean
  loginEffectiveHours: number
  exclude: string[]
  importExclude: boolean
  enabledExclude: boolean
  bgmJpName: boolean
  tmdb: boolean
  tmdbId: boolean
  tmdbIdPlexMode: boolean
  tmdbOriginalName: boolean
  tmdbLanguage: string
  ipWhitelist: boolean
  ipWhitelistStr: string
  omit: boolean
  bgmToken: string
  bgmAppID: string
  bgmAppSecret: string
  bgmRefreshToken: string
  bgmRedirectUri: string
  apiKey: string
  downloadNew: boolean
  renameTemplate: string
  renameDelYear: boolean
  renameDelTmdbId: boolean
  notificationTemplate: string
  bgmImage: string
  customCss: string
  customJs: string
  customEpisode: boolean
  customEpisodeStr: string
  customEpisodeGroupIndex: number
  procrastinating: boolean
  procrastinatingDay: number
  updateTotalEpisodeNumber: boolean
  forceUpdateTotalEpisodeNumber: boolean
  downloadTimeout: number
  notificationConfigList: NotificationConfig[]
  copyMasterToStandby: boolean
  sortType: string
  replace: boolean
  maxFileNameLength: number
  limitLoginAttempts: boolean
  reverseProxyTrustIpList: string[]
  bgmApi: string
  allowCors: boolean
  uuid: string
  // AI 设置
  aiEnabled: boolean
  aiProvider: string
  aiApiKey: string
  aiBaseURL: string
  aiModel: string
  aiPrompt: string
}

export interface Ani {
  id: string
  title: string
  jpTitle: string
  season: number
  subgroup: string
  url: string
  bgmUrl: string
  enable: boolean
  ova: boolean
  cover: string
  image: string
  releaseDate: string
  match: string[]
  exclude: string[]
  currentEpisodeNumber: number
  totalEpisodeNumber: number
  score: number
  offset: number
  sort: number
  type: string
  lastDownloadTime: number
  message: boolean
  downloadNew: boolean
  notDownload: number[]
  customDownloadPath: boolean
  customDownloadPathTemplate: string
  customRenameTemplate: string
  customRenameTemplateEnable: boolean
  customEpisode: boolean
  customEpisodeStr: string
  customEpisodeGroupIndex: number
  omit: boolean
  procrastinating: boolean
  globalExclude: boolean
  tmdb?: {
    id: number
    name: string
    date: string
    tmdbGroupId: string
  } | null
}

export interface WeekAni {
  weekLabel: string
  items: Ani[]
}

export interface ListAniData {
  releaseDateList: string[]
  weekList: WeekAni[]
  total: number
}

export interface Item {
  title: string
  reName: string
  torrent: string
  infoHash: string
  episode: number
  formatSize: string
  length: number
  hasDownloaded: boolean
  master: boolean
  subgroup: string
  pubDate: string
}

export interface PreviewAniData {
  downloadPath: string
  items: Item[]
  omitList: number[]
}

export interface GardenSubject {
  id: string
  name: string
  cover: string
  weekLabel: string
  exists: boolean
}

export interface GardenWeek {
  weekLabel: string
  subjects: GardenSubject[]
}

export interface GardenGroup {
  id: string
  name: string
  rss: string
  bgmId: string
  lastUpdatedAt: string
  items: any[]
}

export interface LoginStatus {
  configured: boolean
  loginOK: boolean
  message: string
}

export interface BgmInfo {
  id: string
  name: string
  nameCn: string
  eps: number
  season: number
  rating: { rank: number; score: number; total: number }
  images: { small: string; large: string; medium: string }
}

export interface RssToAniDTO {
  url: string
  type: string
  bgmUrl?: string
  subgroup?: string
  enable?: boolean
}

export interface LogEntry {
  message: string
  level: string
  loggerName: string
  threadName: string
}

export interface ServiceStatus {
  ai: { configured: boolean; ok: boolean; reply: string; message: string }
  cloud: { configured: boolean; loginOK: boolean; message: string }
  memory: { allocMB: number; totalAllocMB: number; sysMB: number; numGC: number }
  cache: { count: number; bytes: number; sizeKB: number }
  uptimeSeconds: number
}