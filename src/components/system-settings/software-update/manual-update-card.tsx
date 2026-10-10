"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { motion } from "motion/react";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { FileUpIcon, LoaderCircle, PackagePlusIcon, TriangleAlertIcon } from "lucide-react";
import { authFetch } from "@/lib/auth-fetch";
import { toast } from "sonner";

/**
 * Manual Update:
 * Upload qmanager-core-armv7.tar.gz langsung dari UI (tanpa SSH/OTA).
 * Biar bisa dipakai dari HP: pakai <input type="file"> native (bukan
 * hidden input + click() yang sering di-block browser mobile).
 */
export function ManualUpdateCard() {
  const { t } = useTranslation("system-settings");
  const [file, setFile] = useState<File | null>(null);
  const [uploading, setUploading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const onUpload = async () => {
    if (!file || uploading) return;
    setUploading(true);
    setError(null);
    try {
      const fd = new FormData();
      fd.append("file", file);
      const res = await authFetch("/cgi-bin/quecmanager/system/update_upload.sh", {
        method: "POST",
        body: fd,
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok || data.success === false) {
        throw new Error(data.message || `HTTP ${res.status}`);
      }
      toast.success("Update installed - service restarting…", { duration: 4000 });
      setTimeout(() => window.location.reload(), 2500);
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : "Upload failed";
      setError(msg);
      toast.error(msg);
    } finally {
      setUploading(false);
    }
  };

  return (
    <motion.div
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.25 }}
    >
      <Card className="border-dashed">
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <PackagePlusIcon className="size-4 text-muted-foreground" />
            Manual Update (Upload File)
          </CardTitle>
          <CardDescription>
            Pilih file <code>qmanager-core-armv7.tar.gz</code> dari GitHub Releases,
            lalu klik Upload. Tidak perlu SSH.
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          <input
            type="file"
            accept=".tar,.tar.gz,.gz,application/gzip,application/x-tar"
            className="block w-full cursor-pointer rounded-md border border-input bg-background px-3 py-2 text-sm file:mr-3 file:cursor-pointer file:rounded-md file:border-0 file:bg-primary/10 file:px-3 file:py-1.5 file:text-sm file:font-medium file:text-primary hover:file:bg-primary/20"
            onChange={(e) => {
              setFile(e.target.files?.[0] ?? null);
              setError(null);
            }}
          />
          {file && (
            <div className="text-sm text-muted-foreground">
              Dipilih: <span className="font-medium text-foreground">{file.name}</span> (
              {(file.size / 1024 / 1024).toFixed(2)} MB)
            </div>
          )}
          {error && (
            <Alert variant="destructive">
              <TriangleAlertIcon className="size-4" />
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}
          <div className="flex justify-end">
            <Button onClick={onUpload} disabled={!file || uploading}>
              {uploading ? (
                <>
                  <LoaderCircle className="size-4 animate-spin" /> Uploading…
                </>
              ) : (
                <>
                  <FileUpIcon className="size-4" /> {t("software_update.upload_button", "Upload & Install")}
                </>
              )}
            </Button>
          </div>
        </CardContent>
      </Card>
    </motion.div>
  );
}