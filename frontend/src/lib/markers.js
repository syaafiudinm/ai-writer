// Penanda fakta yang belum pasti di draf: [perlu konfirmasi: …] (id) dan
// [needs confirmation: …] (en). Disorot di preview agar editor tidak melewatkannya.

const MARKER = /\[(?:perlu konfirmasi|needs confirmation)(?::[^\]\n]*)?\]/gi;

export function countMarkers(text) {
  return (text.match(MARKER) || []).length;
}

// splitMarkers memecah teks menjadi node mdast; penanda menjadi elemen <mark>.
export function splitMarkers(value) {
  const nodes = [];
  let last = 0;
  for (const m of value.matchAll(MARKER)) {
    if (m.index > last) nodes.push({ type: 'text', value: value.slice(last, m.index) });
    nodes.push({
      type: 'confirmMarker',
      data: { hName: 'mark', hProperties: { className: ['confirm-marker'] } },
      children: [{ type: 'text', value: m[0] }],
    });
    last = m.index + m[0].length;
  }
  if (last === 0) return null;
  if (last < value.length) nodes.push({ type: 'text', value: value.slice(last) });
  return nodes;
}

function walk(node) {
  if (!Array.isArray(node.children)) return;
  const out = [];
  for (const child of node.children) {
    if (child.type === 'text') {
      const parts = splitMarkers(child.value);
      if (parts) out.push(...parts);
      else out.push(child);
    } else {
      if (child.type !== 'code' && child.type !== 'inlineCode') walk(child);
      out.push(child);
    }
  }
  node.children = out;
}

// remarkConfirmMarkers adalah plugin remark untuk react-markdown.
export function remarkConfirmMarkers() {
  return (tree) => walk(tree);
}
