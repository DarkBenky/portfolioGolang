export function formatMarkdown(text) {
  if (!text) return ''

  let html = text

  html = html.replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')

  html = html.replace(/^### (.+)$/gm, '<h4 class="text-subtitle-2 font-weight-bold mt-3 mb-1">$1</h4>')
  html = html.replace(/^## (.+)$/gm, '<h3 class="text-subtitle-1 font-weight-bold mt-3 mb-1">$1</h3>')
  html = html.replace(/^# (.+)$/gm, '<h2 class="text-h6 font-weight-bold mt-3 mb-2">$1</h2>')

  html = html.replace(/\*\*\*(.+?)\*\*\*/g, '<strong><em>$1</em></strong>')
  html = html.replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
  html = html.replace(/\*(.+?)\*/g, '<em>$1</em>')
  html = html.replace(/_(.+?)_/g, '<em>$1</em>')

  html = html.replace(/^[\-\*] (.+)$/gm, '<li>$1</li>')
  html = html.replace(/(<li>.*<\/li>\n?)+/g, '<ul class="ml-4 my-2">$&</ul>')
  html = html.replace(/^\d+\. (.+)$/gm, '<li>$1</li>')

  html = html.replace(/`([^`]+)`/g, '<code class="px-1 rounded" style="background: rgba(var(--v-theme-surface-variant), 0.5);">$1</code>')

  html = html.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank" class="text-primary">$1</a>')

  html = html.replace(/\n\n/g, '</p><p class="mb-2">')
  html = '<p class="mb-2">' + html + '</p>'
  html = html.replace(/\n/g, '<br>')

  html = html.replace(/<p class="mb-2"><\/p>/g, '')
  html = html.replace(/<p class="mb-2">(<h[2-4])/g, '$1')
  html = html.replace(/(<\/h[2-4]>)<\/p>/g, '$1')

  return html
}
