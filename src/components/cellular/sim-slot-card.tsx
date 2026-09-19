"use client";

import React, { useState, useEffect, useCallback } from "react";
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { RefreshCw, ArrowRightLeft, CreditCard, Cpu } from "lucide-react";
import { authFetch } from "@/lib/auth-fetch";
import { toast } from "sonner";
import { SensitiveValue } from "@/components/ui/sensitive-value";

export interface SIMSlotInfo {
  active_slot: number;
  iccid: string;
  status: string;
}

export function SIMSlotCard() {
  const [info, setInfo] = useState<SIMSlotInfo | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [isSwitching, setIsSwitching] = useState(false);

  const fetchSIMInfo = useCallback(async () => {
    setIsLoading(true);
    try {
      const res = await authFetch("/cgi-bin/quecmanager/cellular/sim_slot.sh");
      if (res.ok) {
        const data = await res.json();
        setInfo(data);
      }
    } catch {
      // Quiet fail fallback
    } finally {
      setIsLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchSIMInfo();
  }, [fetchSIMInfo]);

  const handleSwitchSlot = async (targetSlot: number) => {
    if (info?.active_slot === targetSlot) return;

    setIsSwitching(true);
    toast.info(`Switching modem to SIM Slot ${targetSlot}...`);

    try {
      const res = await authFetch("/cgi-bin/quecmanager/cellular/sim_slot.sh", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ slot: targetSlot }),
      });

      if (!res.ok) {
        throw new Error("SIM slot switch failed");
      }

      toast.success(`Successfully switched to SIM Slot ${targetSlot}!`);
      await fetchSIMInfo();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to switch SIM slot");
    } finally {
      setIsSwitching(false);
    }
  };

  return (
    <Card className="w-full">
      <CardHeader>
        <CardTitle className="text-lg font-semibold flex items-center justify-between">
          <span className="flex items-center gap-2">
            <CreditCard className="size-5 text-primary" />
            Dual-SIM & eSIM Management
          </span>
          <Button
            variant="ghost"
            size="sm"
            onClick={fetchSIMInfo}
            disabled={isLoading}
          >
            <RefreshCw className={`size-4 ${isLoading ? "animate-spin" : ""}`} />
          </Button>
        </CardTitle>
        <CardDescription>
          Switch active physical SIM or eSIM slot directly on Quectel modem hardware.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
          {/* SIM Slot 1 */}
          <div
            className={`p-4 rounded-lg border flex flex-col justify-between gap-3 transition-colors ${
              info?.active_slot === 1
                ? "border-primary/50 bg-primary/5"
                : "bg-muted/30"
            }`}
          >
            <div className="flex items-center justify-between">
              <span className="font-semibold text-sm flex items-center gap-2">
                <Cpu className="size-4 text-primary" />
                SIM Slot 1
              </span>
              {info?.active_slot === 1 && (
                <Badge variant="outline" className="bg-emerald-500/15 text-emerald-600 border-emerald-500/30">
                  Active
                </Badge>
              )}
            </div>

            <div className="text-xs text-muted-foreground font-mono truncate">
              ICCID: {info?.active_slot === 1 ? <SensitiveValue value={info.iccid || "Detected"} /> : "Inactive"}
            </div>

            <Button
              size="sm"
              variant={info?.active_slot === 1 ? "outline" : "default"}
              disabled={info?.active_slot === 1 || isSwitching}
              onClick={() => handleSwitchSlot(1)}
              className="w-full gap-2"
            >
              <ArrowRightLeft className="size-3.5" />
              {info?.active_slot === 1 ? "Active Slot" : "Switch to Slot 1"}
            </Button>
          </div>

          {/* SIM Slot 2 */}
          <div
            className={`p-4 rounded-lg border flex flex-col justify-between gap-3 transition-colors ${
              info?.active_slot === 2
                ? "border-primary/50 bg-primary/5"
                : "bg-muted/30"
            }`}
          >
            <div className="flex items-center justify-between">
              <span className="font-semibold text-sm flex items-center gap-2">
                <Cpu className="size-4 text-primary" />
                SIM Slot 2 / eSIM
              </span>
              {info?.active_slot === 2 && (
                <Badge variant="outline" className="bg-emerald-500/15 text-emerald-600 border-emerald-500/30">
                  Active
                </Badge>
              )}
            </div>

            <div className="text-xs text-muted-foreground font-mono truncate">
              ICCID: {info?.active_slot === 2 ? <SensitiveValue value={info.iccid || "Detected"} /> : "Inactive"}
            </div>

            <Button
              size="sm"
              variant={info?.active_slot === 2 ? "outline" : "default"}
              disabled={info?.active_slot === 2 || isSwitching}
              onClick={() => handleSwitchSlot(2)}
              className="w-full gap-2"
            >
              <ArrowRightLeft className="size-3.5" />
              {info?.active_slot === 2 ? "Active Slot" : "Switch to Slot 2"}
            </Button>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
