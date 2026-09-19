"use client";

import { motion } from "motion/react";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { AlertCircle, RefreshCcw } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

import type { AboutDeviceData } from "@/types/about-device";
import { SensitiveValue } from "@/components/ui/sensitive-value";

// =============================================================================
// DeviceInformationCard — Modem image + device identity & network addresses
// =============================================================================

interface DataRow {
  label: string;
  value: string;
  mono?: boolean;
  /** Nilai sensitif (IMEI, dll) - di-blur & di-mask dengan SensitiveValue. */
  sensitive?: boolean;
}

interface DataSection {
  title: string;
  rows: DataRow[];
}

interface DeviceInformationCardProps {
  data: AboutDeviceData | null;
  isLoading: boolean;
  error: string | null;
  onRetry: () => void;
}

function buildSections(data: AboutDeviceData, t: TFunction): DataSection[] {
  return [
    {
      title: t("about_device.device_info.sections.device"),
      rows: [
        { label: t("about_device.device_info.fields.manufacturer_label"), value: data.device.manufacturer },
        { label: t("about_device.device_info.fields.model_label"), value: data.device.model },
        { label: t("about_device.device_info.fields.firmware_label"), value: data.device.firmware },
        { label: t("about_device.device_info.fields.build_date_label"), value: data.device.build_date },
        { label: t("about_device.device_info.fields.imei_label"), value: data.device.imei, mono: true, sensitive: true },
        {
          label: t("about_device.device_info.fields.lte_3gpp_release_label"),
          value: data.threeGppRelease.lte,
        },
        {
          label: t("about_device.device_info.fields.nr5g_3gpp_release_label"),
          value: data.threeGppRelease.nr5g,
        },
      ],
    },
    {
      title: t("about_device.device_info.sections.system"),
      rows: [
        { label: t("about_device.device_info.fields.hostname_label"), value: data.system.hostname },
        {
          label: t("about_device.device_info.fields.openwrt_version_label"),
          value: data.system.openwrt_version,
          mono: true,
        },
        {
          label: t("about_device.device_info.fields.kernel_version_label"),
          value: data.system.kernel_version,
          mono: true,
        },
      ],
    },
  ];
}

// ─── Loading skeleton ────────────────────────────────────────────────────────

function DeviceInformationSkeleton() {
  const { t } = useTranslation("system-settings");
  return (
    <Card className="@container/card">
      <CardHeader>
        <CardTitle className="text-2xl font-semibold">
          {t("about_device.device_info.card_title")}
        </CardTitle>
        <CardDescription>
          {t("about_device.device_info.card_description")}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className="flex items-center justify-center mb-8">
          <Skeleton className="size-44 rounded-full" />
        </div>
        <div className="grid divide-y divide-border border-y border-border">
          {Array.from({ length: 10 }).map((_, i) => (
            <div key={i} className="flex items-center justify-between py-2">
              <Skeleton className="h-4 w-28" />
              <Skeleton className="h-4 w-36" />
            </div>
          ))}
        </div>
      </CardContent>
    </Card>
  );
}

// ─── Main component ──────────────────────────────────────────────────────────

const DeviceInformationCard = ({
  data,
  isLoading,
  error,
  onRetry,
}: DeviceInformationCardProps) => {
  const { t } = useTranslation("system-settings");

  if (isLoading) {
    return <DeviceInformationSkeleton />;
  }

  return (
    <Card className="@container/card">
      <CardHeader>
        <CardTitle className="text-2xl font-semibold">
          {t("about_device.device_info.card_title")}
        </CardTitle>
        <CardDescription>
          {t("about_device.device_info.card_description")}
        </CardDescription>
      </CardHeader>
      <CardContent aria-live="polite">
        {error ? (
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertTitle>{t("about_device.device_info.error.title")}</AlertTitle>
            <AlertDescription className="flex items-center justify-between">
              <span>{error}</span>
              <Button variant="outline" size="sm" onClick={onRetry}>
                <RefreshCcw className="size-3.5 mr-1.5" />
                {t("about_device.device_info.error.retry_button")}
              </Button>
            </AlertDescription>
          </Alert>
        ) : data ? (
          <div className="grid gap-4">
            {/* Modem image */}
            <motion.div
              className="flex items-center justify-center mb-4"
              initial={{ opacity: 0, scale: 0.85 }}
              animate={{ opacity: 1, scale: 1 }}
              transition={{ duration: 0.35, ease: "easeOut" }}
            >
              <div className="size-44 bg-primary/15 rounded-full p-4 flex items-center justify-center">
                <img
                  src="/device-icon.png"
                  alt={
                    data.device.model
                      ? t("about_device.device_info.image_alt", { model: data.device.model })
                      : t("about_device.device_info.image_alt_fallback")
                  }
                  className="size-full drop-shadow-md object-contain"
                />
              </div>
            </motion.div>

            {/* Data sections */}
            {buildSections(data, t).map((section) => (
              <div key={section.title}>
                <h3 className="text-xs font-medium text-muted-foreground uppercase tracking-wider mb-2">
                  {section.title}
                </h3>
                <motion.dl
                  className="grid divide-y divide-border border-y border-border"
                  initial="hidden"
                  animate="visible"
                  variants={{ hidden: {}, visible: { transition: { staggerChildren: 0.04 } } }}
                >
                  {section.rows.map((row) => (
                    <motion.div
                      key={row.label}
                      className="flex items-center justify-between py-2"
                      variants={{ hidden: { opacity: 0, y: 4 }, visible: { opacity: 1, y: 0 } }}
                      transition={{ duration: 0.2, ease: "easeOut" }}
                    >
                      <dt className="text-sm font-semibold text-muted-foreground">
                        {row.label}
                      </dt>
                      <dd
                        className={`text-sm font-semibold min-w-0 truncate ml-4 ${
                          row.mono ? "tabular-nums" : ""
                        }`}
                        title={row.sensitive ? undefined : row.value || undefined}
                      >
                        {row.sensitive ? (
                          <SensitiveValue value={row.value || "-"} />
                        ) : (
                          row.value || "-"
                        )}
                      </dd>
                    </motion.div>
                  ))}
                </motion.dl>
              </div>
            ))}
          </div>
        ) : null}
      </CardContent>
    </Card>
  );
};

export default DeviceInformationCard;
