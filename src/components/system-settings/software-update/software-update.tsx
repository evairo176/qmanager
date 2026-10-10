"use client";

import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { motion } from "motion/react";
import { cn } from "@/lib/utils";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Alert, AlertTitle, AlertDescription } from "@/components/ui/alert";
import {
  CheckIcon,
  CheckCircle2Icon,
  AlertTriangleIcon,
  TriangleAlertIcon,
  DownloadIcon,
  DownloadCloud,
  LoaderCircle,
  RefreshCwIcon,
  PackageIcon,
  RotateCwIcon,
} from "lucide-react";
import { PageHeader } from "@/components/page-header";

import { useSoftwareUpdate } from "@/hooks/use-software-update";
import type { UpdateStatus } from "@/hooks/use-software-update";
import { toast } from "sonner";
import { UpdateStatusCard } from "./update-status-card";
import { UpdatePreferencesCard } from "./update-preferences-card";
import { ManualUpdateCard } from "./manual-update-card";

// ─── Shared helpers ─────────────────────────────────────────────────────────

export function StatusBadge({
  updateAvailable,
  isUpdating,
  isDownloading,
  updateStatus,
}: {
  updateAvailable: boolean;
  isUpdating: boolean;
  isDownloading?: boolean;
  updateStatus: UpdateStatus;
}) {
  const { t } = useTranslation("system-settings");

  if (isUpdating && updateStatus.status !== "error") {
    return (
      <Badge variant="outline" className="bg-info/15 text-info hover:bg-info/20 border-info/30">
        <DownloadIcon className="size-3" />
        {t("software_update.badge_updating")}
      </Badge>
    );
  }
  if (isDownloading) {
    return (
      <Badge variant="outline" className="bg-info/15 text-info hover:bg-info/20 border-info/30">
        <DownloadIcon className="size-3" />
        {t("software_update.badge_downloading")}
      </Badge>
    );
  }
  if (updateAvailable) {
    return (
      <Badge variant="outline" className="bg-warning/15 text-warning hover:bg-warning/20 border-warning/30">
        <TriangleAlertIcon className="size-3" />
        {t("software_update.badge_update_available")}
      </Badge>
    );
  }
  return (
    <Badge variant="outline" className="bg-success/15 text-success hover:bg-success/20 border-success/30">
      <CheckCircle2Icon className="size-3" />
      {t("software_update.badge_up_to_date")}
    </Badge>
  );
}

// ─── Update progress stepper ────────────────────────────────────────────────

interface StepConfig {
  label: string;
  detail: Record<string, string>;
  icon: React.ReactNode;
}

const STEP_MAP: Record<string, number> = {
  downloading: 0,
  installing: 1,
  rebooting: 2,
};

type StepState = "done" | "active" | "pending";

function getStepState(stepIndex: number, activeIndex: number): StepState {
  if (stepIndex < activeIndex) return "done";
  if (stepIndex === activeIndex) return "active";
  return "pending";
}

const stepIconMap: Record<StepState, (defaultIcon: React.ReactNode) => React.ReactNode> = {
  done: () => <CheckIcon className="size-4 text-success" />,
  active: () => <LoaderCircle className="size-4 animate-spin text-info" />,
  pending: (icon) => <span className="text-muted-foreground/50">{icon}</span>,
};

// ─── Component ──────────────────────────────────────────────────────────────

const SoftwareUpdateComponent = () => {
  const { t } = useTranslation("system-settings");
  const hookData = useSoftwareUpdate();
  const {
    updateInfo,
    updateStatus,
    downloadState,
    isLoading,
    isChecking,
    isUpdating,
    isDownloading,
    isInstallStalled,
    error,
  } = hookData;

  const progressListAria = t("software_update.progress_list_aria");

  // Wrapped action handlers that surface a success toast once the underlying
  // request finishes. Failures are already surfaced by the hook's error state
  // and the child cards' error toasts.
  const handleCheckForUpdates = async () => {
    await hookData.checkForUpdates();
    toast.success("Update check complete", { duration: 2500 });
  };

  const handleDownloadUpdate = async (version?: string) => {
    await hookData.downloadUpdate(version);
    toast.success("Update download started", { duration: 2500 });
  };

  const handleInstallStaged = async () => {
    await hookData.installStaged();
    toast.success("Update install started", { duration: 2500 });
  };

  const handleRebootDevice = async () => {
    await hookData.rebootDevice();
    toast.success("Reboot requested", { duration: 2500 });
  };

  const steps = useMemo<StepConfig[]>(
    () => [
      {
        label: t("software_update.steps_download_label"),
        detail: {
          active: t("software_update.steps_download_active"),
          done: t("software_update.steps_download_done"),
        },
        icon: <DownloadIcon className="size-4" />,
      },
      {
        label: t("software_update.steps_install_label"),
        detail: {
          active: t("software_update.steps_install_active"),
          done: t("software_update.steps_install_done"),
        },
        icon: <PackageIcon className="size-4" />,
      },
      {
        label: t("software_update.steps_reboot_label"),
        detail: {
          active: t("software_update.steps_reboot_active"),
          done: t("software_update.steps_reboot_done"),
        },
        icon: <RotateCwIcon className="size-4" />,
      },
    ],
    [t],
  );

  const waitingLabel = t("software_update.steps_waiting");

  // ── Fatal error (no data at all) ──────────────────────────────────────
  if (error && !updateInfo && !isLoading) {
    return (
      <PageWrapper>
        <Card className="@container/card">
          <CardHeader>
            <CardTitle>{t("software_update.status_card_title")}</CardTitle>
            <CardDescription>
              {t("software_update.error_unable_to_check")}
            </CardDescription>
          </CardHeader>
          <CardContent>
            <Alert variant="destructive">
              <AlertTriangleIcon className="size-4" />
              <AlertTitle>{t("software_update.error_failed_title")}</AlertTitle>
              <AlertDescription>
                <p>{error}</p>
              </AlertDescription>
            </Alert>
            <div className="mt-4 flex justify-end">
              <Button
                variant="outline"
                onClick={handleCheckForUpdates}
                disabled={isChecking}
              >
                {isChecking ? (
                  <>
                    <LoaderCircle className="size-4 animate-spin" />
                    {t("software_update.checking")}
                  </>
                ) : (
                  <>
                    <RefreshCwIcon className="size-4" />
                    {t("actions.retry", { ns: "common" })}
                  </>
                )}
              </Button>
            </div>
          </CardContent>
        </Card>
      </PageWrapper>
    );
  }

  // ── Updating state (replaces entire card grid) ────────────────────────
  if (isUpdating && updateStatus.status !== "error") {
    const activeIndex = STEP_MAP[updateStatus.status] ?? 0;

    return (
      <PageWrapper>
        <Card className="@container/card">
          <CardHeader>
            <CardTitle>{t("software_update.updating_card_title")}</CardTitle>
            <CardDescription>
              {updateStatus.version
                ? t("software_update.updating_description_version", { version: updateStatus.version })
                : t("software_update.updating_description_generic")}
              {updateStatus.size && ` (${updateStatus.size})`}
            </CardDescription>
            <CardAction>
              <StatusBadge
                updateAvailable={false}
                isUpdating={true}
                isDownloading={isDownloading}
                updateStatus={updateStatus}
              />
            </CardAction>
          </CardHeader>
          <CardContent>
            <div className="flex flex-col gap-5" aria-live="polite">
              {/* Step list */}
              <div className="space-y-1" role="list" aria-label={progressListAria}>
                {steps.map((step, i) => {
                  const state = getStepState(i, activeIndex);
                  const detailText =
                    state === "active"
                      ? updateStatus.message || step.detail.active
                      : state === "done"
                        ? step.detail.done
                        : waitingLabel;

                  return (
                    <motion.div
                      key={step.label}
                      initial={{ opacity: 0, x: -8 }}
                      animate={{ opacity: 1, x: 0 }}
                      transition={{ duration: 0.25, delay: i * 0.06, ease: "easeOut" }}
                      role="listitem"
                      aria-current={state === "active" ? "step" : undefined}
                      className={cn(
                        "flex items-start gap-3 rounded-lg px-3 py-2.5 text-sm transition-colors duration-300",
                        state === "active" && "bg-info/5",
                      )}
                    >
                      <div className="mt-0.5 shrink-0">
                        {stepIconMap[state](step.icon)}
                      </div>
                      <div className="flex-1 min-w-0">
                        <p
                          className={cn(
                            "font-medium",
                            state === "done" && "text-success",
                            state === "active" && "text-foreground",
                            state === "pending" && "text-muted-foreground",
                          )}
                        >
                          {step.label}
                        </p>
                        <p
                          className={cn(
                            "text-xs mt-0.5",
                            state === "active"
                              ? "text-muted-foreground"
                              : "text-muted-foreground/60",
                          )}
                        >
                          {detailText}
                        </p>
                      </div>
                    </motion.div>
                  );
                })}
              </div>

              {/* Segmented progress bar */}
              <div
                className="flex w-full gap-1.5"
                role="progressbar"
                aria-valuenow={activeIndex + 1}
                aria-valuemax={steps.length}
                aria-label={progressListAria}
              >
                {steps.map((step, i) => (
                  <div
                    key={step.label}
                    className={cn(
                      "h-0.75 flex-1 rounded-full transition-colors duration-500",
                      i < activeIndex
                        ? "bg-success"
                        : i === activeIndex
                          ? "bg-primary/60"
                          : "bg-muted/30",
                    )}
                  />
                ))}
              </div>

              {/* Warning footer */}
              <div role="alert" className="flex items-center justify-center gap-2 rounded-lg bg-warning/10 px-4 py-2.5">
                <AlertTriangleIcon className="size-4 shrink-0 text-warning" />
                <p className="text-xs font-medium text-warning">
                  {t("software_update.updating_do_not_power_off")}
                </p>
              </div>

              {isInstallStalled && updateStatus.status === "installing" && (
                <Alert variant="warning">
                  <AlertTriangleIcon className="size-4" />
                  <AlertTitle>{t("software_update.updating_stall_title")}</AlertTitle>
                  <AlertDescription>
                    <p className="mb-3">
                      {t("software_update.updating_stall_description")}
                    </p>
                    <Button size="sm" onClick={handleRebootDevice}>
                      {t("software_update.updating_reboot_device_now")}
                    </Button>
                  </AlertDescription>
                </Alert>
              )}
            </div>
          </CardContent>
        </Card>
      </PageWrapper>
    );
  }

  // ── Normal state: 2-card grid ─────────────────────────────────────────
  return (
    <PageWrapper>
      <div className="grid grid-cols-1 @3xl/main:grid-cols-2 grid-flow-row gap-4">
        <UpdateStatusCard
          updateInfo={updateInfo}
          updateStatus={updateStatus}
          downloadState={downloadState}
          isLoading={isLoading}
          isChecking={isChecking}
          isUpdating={isUpdating}
          isDownloading={isDownloading}
          error={error}
          lastChecked={hookData.lastChecked}
          checkForUpdates={handleCheckForUpdates}
          downloadUpdate={handleDownloadUpdate}
          installStaged={handleInstallStaged}
        />
        <UpdatePreferencesCard
          updateInfo={updateInfo}
          isLoading={isLoading}
          isUpdating={isUpdating}
          isDownloading={isDownloading}
          downloadUpdate={hookData.downloadUpdate}
          togglePrerelease={hookData.togglePrerelease}
          saveAutoUpdate={hookData.saveAutoUpdate}
        />
      </div>
      <ManualUpdateCard />
    </PageWrapper>
  );
};

function PageWrapper({ children }: { children: React.ReactNode }) {
  const { t } = useTranslation("system-settings");
  return (
    <div className="@container/main mx-auto flex flex-col gap-6">
      <PageHeader
        icon={DownloadCloud}
        title={t("software_update.page_title")}
        description={t("software_update.page_description")}
      />
      {children}
    </div>
  );
}

export default SoftwareUpdateComponent;
