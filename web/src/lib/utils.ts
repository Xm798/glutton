import { clsx, type ClassValue } from "clsx"
import { twMerge } from "tailwind-merge"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

const UNITS = ["B", "KB", "MB", "GB", "TB", "PB"];
export function formatBytes(n: number, digits = 1): string {
  if (!n || !isFinite(n)) return "0 B";
  const i = Math.min(Math.floor(Math.log(Math.abs(n)) / Math.log(1024)), UNITS.length - 1);
  return `${(n / Math.pow(1024, i)).toFixed(digits)} ${UNITS[i]}`;
}

export function formatRate(bps: number): string {
  return `${formatBytes(bps)}/s`;
}
