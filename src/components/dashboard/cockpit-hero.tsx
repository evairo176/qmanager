"use client";

import React from "react";
import { motion } from "motion/react";
import { RadioTower, Activity, Cpu, Thermometer, ShieldCheck, Wifi, Sparkles, Layers } from "lucide-react";
import { cn } from "@/lib/utils";
import { containerVariants, itemVariants } from "@/lib/motion";
import type { ModemStatus } from "@/types/modem-status";

interface CockpitHeroProps {
  data: ModemStatus | null;
  isLoading: boolean;
}

export function CockpitHero({ data, isLoading }: CockpitHeroProps) {
  const isOnline = data?.modem_reachable && data?.network?.type;
  const networkType = data?.network?.type || "CELLULAR";
  const carrier = data?.network?.carrier || "Unknown Network";
  const wanIp = data?.network?.wan_ipv4 || null;
  const caActive = data?.network?.ca_active;
  // Count & bandwidth come from the real CA carrier list when available, so the
  // hero matches the Active Carriers panel (e.g. 3x CA, LTE 35 MHz total).
  const carrierComps = data?.network?.carrier_components ?? [];
  const caCount =
    carrierComps.length > 0
      ? carrierComps.length
      : (data?.network?.ca_count || 0) + 1;
  // Aggregated bandwidth = sum of LTE component bandwidths (NR bw unknown).
  const totalBw =
    carrierComps.length > 0
      ? carrierComps.reduce(
          (sum, c) => sum + (c.bandwidth_mhz || 0),
          0,
        )
      : (data?.lte?.bandwidth ?? 0);

  const latency = data?.connectivity?.latency_ms;
  const packetLoss = data?.connectivity?.packet_loss_pct ?? 0;
  const temp = data?.device?.temperature;
  const model = data?.device?.model || "Quectel 5G";

  const is5G = networkType.includes("5G") || networkType.includes("NR");

  return (
    <div className="relative col-span-full overflow-hidden rounded-2xl border border-border/70 bg-gradient-to-br from-card/80 via-card/50 to-muted/20 backdrop-blur-xl p-5 shadow-xs transition-all duration-300 hover:border-border">
      {/* GSAP Web style subtle ambient mesh highlight */}
      <div className="pointer-events-none absolute -right-16 -top-16 h-48 w-48 rounded-full bg-primary/15 blur-3xl" />
      <div className="pointer-events-none absolute -left-16 -bottom-16 h-48 w-48 rounded-full bg-blue-500/10 blur-3xl" />

      <div className="relative z-10 flex flex-col lg:flex-row lg:items-center lg:justify-between gap-5">
        {/* Left: Device & Operator Brand Badge */}
        <div className="flex items-center gap-4">
          <div className={cn(
            "flex size-12 shrink-0 items-center justify-center rounded-xl border shadow-xs transition-transform duration-300 hover:scale-105",
            is5G
              ? "bg-primary/10 border-primary/30 text-primary"
              : "bg-emerald-500/10 border-emerald-500/30 text-emerald-500"
          )}>
            <RadioTower className="size-6 animate-pulse" />
          </div>

          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="text-xl font-bold tracking-tight text-foreground truncate">
                {carrier}
              </h2>
              <span className={cn(
                "inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-bold font-mono tracking-tight shadow-xs",
                is5G
                  ? "bg-primary text-primary-foreground shadow-primary/20"
                  : "bg-emerald-500 text-white shadow-emerald-500/20"
              )}>
                <Sparkles className="size-3" />
                {networkType}
              </span>
              {caActive && (
                <span className="inline-flex items-center gap-1 rounded-full border border-blue-500/30 bg-blue-500/10 px-2 py-0.5 text-[11px] font-semibold text-blue-500 dark:text-blue-400 font-mono">
                  <Layers className="size-3" />
                  {caCount}x CA ({totalBw} MHz)
                </span>
              )}
            </div>

            <div className="mt-1 flex flex-wrap items-center gap-3 text-xs text-muted-foreground">
              <span className="flex items-center gap-1">
                <Cpu className="size-3.5" />
                <span className="font-medium text-foreground/80">{model}</span>
              </span>
              {wanIp && (
                <>
                  <span className="text-muted-foreground/40">•</span>
                  <span className="font-mono bg-muted/60 px-2 py-0.5 rounded text-[11px] text-foreground/90 font-medium">
                    WAN {wanIp}
                  </span>
                </>
              )}
            </div>
          </div>
        </div>

        {/* Right: Telemetry Quick Gauges */}
        <motion.div
          variants={containerVariants}
          initial="hidden"
          animate="visible"
          className="grid grid-cols-2 sm:grid-cols-4 gap-2.5 sm:gap-3 w-full lg:w-auto"
        >
          {/* 1. Connection Status */}
          <motion.div variants={itemVariants} className="rounded-xl border border-border/50 bg-background/50 backdrop-blur-md px-3 py-2.5 shadow-2xs">
            <div className="flex items-center justify-between text-muted-foreground text-[11px] font-medium">
              <span>Link Status</span>
              <Wifi className="size-3 text-emerald-500" />
            </div>
            <div className="mt-1 flex items-center gap-1.5">
              <span className={cn(
                "size-2 rounded-full",
                isOnline ? "bg-emerald-500 animate-pulse" : "bg-destructive"
              )} />
              <span className="text-sm font-semibold tracking-tight text-foreground font-mono">
                {isOnline ? "CONNECTED" : isLoading ? "CONNECTING" : "OFFLINE"}
              </span>
            </div>
          </motion.div>

          {/* 2. Live Latency */}
          <motion.div variants={itemVariants} className="rounded-xl border border-border/50 bg-background/50 backdrop-blur-md px-3 py-2.5 shadow-2xs">
            <div className="flex items-center justify-between text-muted-foreground text-[11px] font-medium">
              <span>Ping RTT</span>
              <Activity className="size-3 text-blue-500" />
            </div>
            <div className="mt-1 flex items-baseline gap-1">
              <span className="text-sm font-semibold tracking-tight text-foreground font-mono">
                {latency !== null && latency !== undefined ? `${latency}` : "--"}
              </span>
              <span className="text-[10px] text-muted-foreground font-mono">ms</span>
              {packetLoss > 0 && (
                <span className="ml-auto text-[10px] text-destructive font-mono font-medium">
                  {packetLoss}% loss
                </span>
              )}
            </div>
          </motion.div>

          {/* 3. Aggregated Bandwidth */}
          <motion.div variants={itemVariants} className="rounded-xl border border-border/50 bg-background/50 backdrop-blur-md px-3 py-2.5 shadow-2xs">
            <div className="flex items-center justify-between text-muted-foreground text-[11px] font-medium">
              <span>Bandwidth</span>
              <Layers className="size-3 text-purple-500" />
            </div>
            <div className="mt-1 flex items-baseline gap-1">
              <span className="text-sm font-semibold tracking-tight text-foreground font-mono">
                {totalBw > 0 ? `${totalBw}` : "--"}
              </span>
              <span className="text-[10px] text-muted-foreground font-mono">MHz</span>
            </div>
          </motion.div>

          {/* 4. Device Thermal */}
          <motion.div variants={itemVariants} className="rounded-xl border border-border/50 bg-background/50 backdrop-blur-md px-3 py-2.5 shadow-2xs">
            <div className="flex items-center justify-between text-muted-foreground text-[11px] font-medium">
              <span>Modem Temp</span>
              <Thermometer className={cn(
                "size-3",
                temp && temp >= 60 ? "text-destructive" : temp && temp >= 45 ? "text-amber-500" : "text-emerald-500"
              )} />
            </div>
            <div className="mt-1 flex items-baseline gap-1">
              <span className="text-sm font-semibold tracking-tight text-foreground font-mono">
                {temp !== null && temp !== undefined ? `${temp}` : "--"}
              </span>
              <span className="text-[10px] text-muted-foreground font-mono">°C</span>
              <span className={cn(
                "ml-auto text-[10px] font-mono font-medium",
                temp && temp >= 60 ? "text-destructive" : temp && temp >= 45 ? "text-amber-500" : "text-emerald-500"
              )}>
                {temp && temp >= 60 ? "Hot" : temp && temp >= 45 ? "Warm" : "Optimal"}
              </span>
            </div>
          </motion.div>
        </motion.div>
      </div>
    </div>
  );
}
