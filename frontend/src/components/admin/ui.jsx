import { useEffect, useRef } from 'react';

// Status selalu tampil dengan ikon + label, tidak hanya warna.
export const STATUS = {
  success: { icon: '✓', label: 'Sukses' },
  error: { icon: '✕', label: 'Error' },
  timeout: { icon: '⧗', label: 'Timeout' },
  cancelled: { icon: '■', label: 'Dibatalkan' },
};

export function StatusBadge({ status }) {
  const s = STATUS[status] || { icon: '?', label: status };
  return (
    <span className={`status status--${status}`}>
      <span aria-hidden="true" className="status__icon">
        {s.icon}
      </span>
      {s.label}
    </span>
  );
}

export function Pagination({ page, totalPages, total, onPage, disabled }) {
  if (!total) return null;
  return (
    <nav className="pagination" aria-label="Halaman">
      <button type="button" className="btn btn--sm" onClick={() => onPage(page - 1)} disabled={disabled || page <= 1}>
        ← Sebelumnya
      </button>
      <span className="pagination__info">
        Halaman {page} dari {Math.max(totalPages, 1)} · {total.toLocaleString('id-ID')} baris
      </span>
      <button
        type="button"
        className="btn btn--sm"
        onClick={() => onPage(page + 1)}
        disabled={disabled || page >= totalPages}
      >
        Berikutnya →
      </button>
    </nav>
  );
}

export function ErrorAlert({ error, onRetry }) {
  if (!error) return null;
  return (
    <div className="alert alert--error" role="alert">
      <span>{error.message}</span>
      {onRetry && (
        <button type="button" className="btn btn--sm" onClick={onRetry}>
          Coba lagi
        </button>
      )}
    </div>
  );
}

// Modal memakai <dialog> bawaan: fokus terkunci dan Esc menutup.
export function Modal({ open, title, onClose, children }) {
  const ref = useRef(null);
  useEffect(() => {
    const d = ref.current;
    if (!d) return;
    if (open && !d.open) d.showModal();
    if (!open && d.open) d.close();
  }, [open]);
  return (
    <dialog
      ref={ref}
      className="modal"
      onClose={onClose}
      onCancel={(e) => {
        e.preventDefault();
        onClose();
      }}
      aria-labelledby="modal-title"
    >
      {open && (
        <>
          <div className="modal__head">
            <h2 id="modal-title" className="card__title">
              {title}
            </h2>
            <button type="button" className="btn btn--sm btn--ghost" onClick={onClose} aria-label="Tutup">
              ✕
            </button>
          </div>
          {children}
        </>
      )}
    </dialog>
  );
}
