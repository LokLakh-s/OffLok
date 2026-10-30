"""OffLok's icons, from its marks in assets/brand/: the taskbar's four states from the bold mark (on: the mark;
fallback and error: an orange or red dot at the bottom right; off: the mark in grey, chosen on 29/10/2026), the app
icon in the desktop's sizes (the mark alone on transparency, 94 % of the square), and the window's mark.
Run from the repository: python3 scripts/brand-icons.py"""
import numpy as np
from PIL import Image, ImageDraw

MARK, SMALL = "assets/brand/offlok-mark.png", "assets/brand/offlok-mark-small.png"
ORANGE, RED = (249, 115, 22), (239, 68, 68)


def badge(img: Image.Image, colour) -> Image.Image:
    """A dot at the bottom right; around it, the mark is cut away so the taskbar's own colour rings the dot."""
    s = img.width
    r, c = s * 0.2, s * 0.78
    hole = Image.new("L", img.size, 0)
    ImageDraw.Draw(hole).ellipse((c - r * 1.35, c - r * 1.35, c + r * 1.35, c + r * 1.35), fill=255)
    a = np.asarray(img).copy()
    a[..., 3] = np.where(np.asarray(hole) > 0, 0, a[..., 3])
    out = Image.fromarray(a, "RGBA")
    ImageDraw.Draw(out).ellipse((c - r, c - r, c + r, c + r), fill=colour + (255,))
    return out


def grey(img: Image.Image) -> Image.Image:
    a = np.asarray(img).astype(float)
    g = a[..., :3] @ np.array([0.3, 0.59, 0.11])
    a[..., :3] = (g * 0.55 + 70)[..., None]
    return Image.fromarray(a.clip(0, 255).astype(np.uint8), "RGBA")


def padded(img: Image.Image, size: int, fill=0.94) -> Image.Image:
    inner = round(size * fill)
    out = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    m = img.resize((inner, inner), Image.LANCZOS)
    out.paste(m, ((size - inner) // 2, (size - inner) // 2), m)
    return out


if __name__ == "__main__":
    # The tray takes one picture and scales it: 64 px drawn from 1024, the dot drawn at that size.
    small = Image.open(SMALL).convert("RGBA").resize((256, 256), Image.LANCZOS)
    for name, img in {"on": small, "fallback": badge(small, ORANGE), "error": badge(small, RED), "off": grey(small)}.items():
        img.resize((64, 64), Image.LANCZOS).save(f"desktop/icons/tray-{name}.png", optimize=True)
    mark = Image.open(MARK).convert("RGBA")
    for size in (16, 24, 32, 48, 64, 128, 256, 512):
        src = Image.open(SMALL).convert("RGBA") if size <= 32 else mark
        padded(src, size).save(f"packaging/linux/icons/offlok-{size}.png", optimize=True)
    mark.resize((256, 256), Image.LANCZOS).save("desktop/frontend/img/offlok-mark.png", optimize=True)
    # Windows: one .ico, each size drawn from the right mark (the bold one up to 32 px).
    sizes = [256, 128, 64, 48, 32, 24, 16]
    frames = [padded(Image.open(SMALL).convert("RGBA") if n <= 32 else mark, n) for n in sizes]
    frames[0].save("packaging/windows/offlok.ico", format="ICO", sizes=[(n, n) for n in sizes], append_images=frames[1:])
    print("desktop/icons/tray-*.png, packaging/linux/icons/offlok-*.png, desktop/frontend/img/offlok-mark.png, packaging/windows/offlok.ico")
