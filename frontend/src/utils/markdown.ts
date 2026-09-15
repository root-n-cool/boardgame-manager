import MarkdownIt from 'markdown-it'

// html:false escapa qualunque HTML grezzo nel testo, quindi il risultato
// è sicuro da inserire con v-html senza bisogno di un sanitizer come
// DOMPurify. breaks:true trasforma un singolo a capo in <br>: le
// descrizioni già salvate (BGG, admin) sono testo semplice con a capo
// singoli, e senza questa opzione sparirebbero nel rendering markdown
// standard (che li ignora finché non sono due).
// Nessuna annotazione di tipo esplicita su `md`: i typing di markdown-it
// esportano un const unito a un'interfaccia con lo stesso nome via
// `export =`, che l'import di default non ripropone come tipo utilizzabile
// per nome — l'inferenza da `new MarkdownIt(...)` funziona, nominarlo no.
const md = new MarkdownIt({
  html: false,
  linkify: true,
  breaks: true,
})

// I link (dalla libreria o da linkify) si aprono in una nuova scheda:
// sono quasi sempre riferimenti a BGG o a risorse esterne al gioco/evento.
type LinkOpenRule = NonNullable<typeof md.renderer.rules.link_open>
const defaultLinkOpen: LinkOpenRule =
  md.renderer.rules.link_open ||
  ((tokens, idx, options, _env, self) => self.renderToken(tokens, idx, options))

md.renderer.rules.link_open = (tokens, idx, options, env, self) => {
  const token = tokens[idx]
  token.attrSet('target', '_blank')
  token.attrSet('rel', 'noopener noreferrer')
  return defaultLinkOpen(tokens, idx, options, env, self)
}

export function renderMarkdown(source: string): string {
  return md.render(source)
}
