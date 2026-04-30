// Mirrors internal/store.Source. PascalCase matches GORM's default JSON output.
export interface Source {
  ID: number;
  Name: string;
  URL: string;
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
  url: string;
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

export interface ServerEvent {
  TS: string; // RFC3339
  Type: string;
  Level: "info" | "warn" | "error" | string;
  Message: string;
  Data?: Record<string, unknown>;
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
