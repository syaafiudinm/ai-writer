import { describe, expect, it } from 'vitest';
import { render } from '@testing-library/react';
import ArticlePreview from '../components/ArticlePreview.jsx';
import { countMarkers } from './markers.js';

describe('penanda konfirmasi', () => {
  it('menghitung penanda id dan en', () => {
    expect(countMarkers('A [perlu konfirmasi: nama] B [needs confirmation: date] [Perlu Konfirmasi]')).toBe(3);
    expect(countMarkers('[perlu konfirmasi: belum ditutup')).toBe(0);
  });

  it('disorot sebagai <mark> di dalam markdown, tidak di blok kode', () => {
    const md = '# Judul\n\nWali kota [perlu konfirmasi: nama] meresmikan **taman [needs confirmation: luas]**.\n\n```\n[perlu konfirmasi: kode]\n```';
    const { container } = render(<ArticlePreview text={md} />);
    const marks = [...container.querySelectorAll('mark.confirm-marker')].map((m) => m.textContent);
    expect(marks).toEqual(['[perlu konfirmasi: nama]', '[needs confirmation: luas]']);
    expect(container.querySelector('strong mark')).not.toBeNull();
  });
});
