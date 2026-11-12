"use client";

import { useId, useState, type DragEvent } from "react";

const size = (bytes: number) =>
  bytes < 1024 * 1024 ? `${Math.max(1, Math.round(bytes / 1024))} KB` : `${(bytes / (1024 * 1024)).toFixed(1)} MB`;

// FilePicker is a drop zone for one file: drag a file onto it, or click (or
// press Enter or Space on) it to choose one. The native input stays in the
// page, hidden from sight but not from the keyboard or screen readers.
export function FilePicker({
  label,
  accept,
  file,
  onFile,
  disabled = false,
}: {
  label: string;
  accept: string;
  file: File | null;
  onFile: (file: File | null) => void;
  disabled?: boolean;
}) {
  const id = useId();
  const [over, setOver] = useState(false);

  const onDrop = (e: DragEvent) => {
    e.preventDefault();
    setOver(false);
    if (disabled) return;
    const dropped = e.dataTransfer.files?.[0];
    if (dropped) onFile(dropped);
  };

  return (
    <div className="file-picker">
      <input
        id={id}
        className="file-picker-input"
        type="file"
        accept={accept}
        disabled={disabled}
        onChange={(e) => onFile(e.target.files?.[0] ?? null)}
      />
      <label
        htmlFor={id}
        className="file-picker-zone"
        data-over={over || undefined}
        data-chosen={file ? true : undefined}
        onDragOver={(e) => {
          e.preventDefault();
          if (!disabled) setOver(true);
        }}
        onDragLeave={() => setOver(false)}
        onDrop={onDrop}
      >
        <span className="file-picker-icon" aria-hidden="true">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round">
            {file ? (
              <>
                <path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z" />
                <path d="M14 3v5h5" />
                <path d="M9 14l2 2 4-4" />
              </>
            ) : (
              <>
                <path d="M12 15V4" />
                <path d="M7.5 8.5L12 4l4.5 4.5" />
                <path d="M5 14v4a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2v-4" />
              </>
            )}
          </svg>
        </span>
        {file ? (
          <span className="file-picker-text">
            <strong>{file.name}</strong>
            <span>
              {size(file.size)}. Drop another file or click to change it.
            </span>
          </span>
        ) : (
          <span className="file-picker-text">
            <strong>{label}</strong>
            <span>Drop it here, or click to choose it.</span>
          </span>
        )}
      </label>
    </div>
  );
}
