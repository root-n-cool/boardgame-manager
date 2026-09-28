/** L'invito sotto il QR di un gioco: nella scatola o nel foglio di tutti. */
export const GAME_QR_INVITE = 'Inquadra per regole, tutorial e classifica'

/** Il codice come `src` di un'immagine: l'SVG arriva già pronto dal backend. */
export const qrDataUrl = (svg: string) => `data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`

/** L'indirizzo in chiaro sotto il codice, senza schema: è il ripiego per
 *  chi il QR non lo legge, e "https://" non si scrive a mano. */
export const printableUrl = (url: string) => url.replace(/^https?:\/\//, '')

/** Lato minimo del JPG: abbastanza per un volantino o una slide. */
const JPG_MIN_SIDE = 1024

/**
 * Scarica il solo codice come JPG, senza scritte: per volantini, social o
 * slide, dove il cartellino non serve. Il lato è un multiplo esatto dei
 * moduli, così ogni quadratino copre gli stessi pixel e i bordi restano
 * netti anche dopo la compressione.
 */
export async function downloadQrJpg(svg: string, title: string): Promise<void> {
  const modules = Number(/viewBox="0 0 (\d+)/.exec(svg)?.[1]) || 1
  const side = modules * Math.ceil(JPG_MIN_SIDE / modules)
  // Senza larghezza e altezza esplicite un SVG non ha misura intrinseca e
  // il canvas lo disegnerebbe a 300×150.
  const sized = svg.replace('<svg ', `<svg width="${side}" height="${side}" `)

  const img = new Image()
  img.src = qrDataUrl(sized)
  await img.decode()

  const canvas = document.createElement('canvas')
  canvas.width = canvas.height = side
  const ctx = canvas.getContext('2d')
  if (!ctx) throw new Error('Il browser non riesce a generare l\'immagine.')
  ctx.imageSmoothingEnabled = false
  ctx.fillStyle = '#fff'
  ctx.fillRect(0, 0, side, side)
  ctx.drawImage(img, 0, 0, side, side)

  const blob = await new Promise<Blob | null>((resolve) => canvas.toBlob(resolve, 'image/jpeg', 0.95))
  if (!blob) throw new Error('Il browser non riesce a generare l\'immagine.')
  const a = document.createElement('a')
  a.href = URL.createObjectURL(blob)
  a.download = `${slugify(title) || 'qr'}-qr.jpg`
  a.click()
  URL.revokeObjectURL(a.href)
}

/** "Città & Cavalieri" → "citta-cavalieri". */
function slugify(s: string): string {
  return s
    .normalize('NFD')
    .replace(/[̀-ͯ]/g, '')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-|-$/g, '')
}
