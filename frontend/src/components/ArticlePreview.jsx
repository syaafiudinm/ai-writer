import { memo } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { remarkConfirmMarkers } from '../lib/markers.js';

const remarkPlugins = [remarkGfm, remarkConfirmMarkers];

// Link dari model dibuka di tab baru dan tanpa referrer.
const components = {
  a: ({ node, ...props }) => <a {...props} target="_blank" rel="noopener noreferrer nofollow" />,
};

function ArticlePreview({ text, streaming, ref }) {
  return (
    <article ref={ref} className={`article${streaming ? ' is-streaming' : ''}`} aria-busy={streaming}>
      <ReactMarkdown remarkPlugins={remarkPlugins} components={components}>
        {text}
      </ReactMarkdown>
    </article>
  );
}

export default memo(ArticlePreview);
