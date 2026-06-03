// Mirrors internal/store.Source. PascalCase matches GORM's default JSON output.
export interface Source {
  ID: number;
  Name: string;
  URLs: string[];
  UA: string;
  Enabled: boolean;
  Weight: number;
  SuccessCount: number;
  FailCount: number;
  AvgSpeedBps: number;
  LastError: string;
  LastSuccessAt: number;
  CooldownUntil: number;
  CreatedAt: number;
  UpdatedAt: number;
}

export interface SourceInput {
  name: string;
  urls: string[];
  ua?: string;
  enabled: boolean;
  weight: number;
}

export interface LiveStats {
  current_rate_bps: number;
  today_bytes: number;
  month_bytes: number;
  updated_at: number;
}

export interface TrafficBucket {
  HourBucket: number;
  SourceID: number;
  Bytes: number;
}

export type TrafficRange = "1h" | "1d" | "1w" | "1m";

export interface TrafficSeries {
  step: number; // seconds per bucket
  points: { t: number; bytes: number }[];
}

export interface SourceTrafficSummary {
  id: number;
  name: string;
  bytes: number;
}

export interface VersionInfo {
  version: string;
  commit: string;
  date: string;
}

// Live SSE event payload (TS is RFC3339 timestamp).
export interface ServerEvent {
  id: number; // monotonic publish id (0 if missing on legacy frames)
  TS: string;
  Type: string;
  Level: "info" | "warn" | "error" | string;
  Message: string;
  Data?: Record<string, unknown>;
}

// Persisted event row from GET /api/events/history. Matches internal/store.Event
// (PascalCase via GORM JSON output).
export interface EventHistoryItem {
  id: number; // monotonic publish id (matches ServerEvent.id; same key space)
  Ts: number;
  Level: string;
  Type: string;
  Message: string;
}

// Runtime config — values are JSON-encoded server-side, but the GET endpoint
// returns the parsed shape, so this matches the parsed JSON.
export interface RuntimeConfig {
  daily_quota_gb?: number;
  monthly_quota_gb?: number;
  max_rate_mbps?: number;
  max_concurrent?: number;
  time_windows?: string[];
  default_ua?: string;
  notifier_urls?: string[];
  subscribed_events?: string[];
}

export type SchedulerStatus = "idle" | "running" | "paused" | "quota_reached";

export interface ControlStatus {
  status: SchedulerStatus;
  burst_active: boolean;
}

// Catalog of event types known by the backend, plus the subset enabled by
// default on a fresh install. Pulled at runtime by the settings UI to avoid
// FE/BE drift (see M-4 in the round-3 review).
export interface EventTypesCatalog {
  all: string[];
  default: string[];
}
