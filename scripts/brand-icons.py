"""OffLok's icons, from its marks in assets/brand/: the taskbar's four states from the bold mark (on: the mark;
fallback and error: an orange or red dot at the bottom right; off: the mark in grey, chosen on 29/10/2026), the app
icon in the desktop's sizes (the mark alone on transparency, 94 % of the square), the Mac's icon on a tile (the
Dock expects one, and macOS 26 shuts a round icon in a grey square: very dark teal, the mark at 72 %, chosen on
31/10/2026), the window's mark, and the website's images.
Run from the repository: python3 scripts/brand-icons.py"""
import os

import numpy as np
from PIL import Image, ImageDraw, ImageFilter

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


def squircle(size: int, n: float = 5.0, ss: int = 4) -> Image.Image:
    """Apple's continuous-corner shape, as a superellipse, antialiased by supersampling."""
    t = (np.arange(size * ss) + 0.5) / (size * ss) * 2 - 1
    x, y = np.meshgrid(t, t)
    inside = (np.abs(x) ** n + np.abs(y) ** n) <= 1
    return Image.fromarray((inside * 255).astype(np.uint8), "L").resize((size, size), Image.LANCZOS)


def mac_tile(src: Image.Image) -> Image.Image:
    """The Mac's icon, 1024 px on Apple's grid: an 824 px tile with a drop shadow, very dark teal (the inside of the
    mark's ring) lit at its centre, a thin teal rim, the mark at 72 % of the tile."""
    S, B = 1024, 824
    t = np.linspace(-1, 1, B)
    x, y = np.meshgrid(t, t)
    r = np.clip(np.sqrt(x ** 2 + (y + 0.1) ** 2) / 1.25, 0, 1)[..., None]
    body = Image.fromarray((np.array([14, 58, 58]) * (1 - r) + np.array([3, 13, 14]) * r).astype(np.uint8), "RGB").convert("RGBA")
    m = src.resize((round(B * 0.72),) * 2, Image.LANCZOS)
    body.alpha_composite(m, ((B - m.width) // 2, (B - m.height) // 2))
    shape = squircle(B)
    edge = np.asarray(shape, int) - np.asarray(shape.filter(ImageFilter.MinFilter(7)), int)
    rim = Image.new("RGBA", (B, B), (110, 231, 214, 0))
    rim.putalpha(Image.fromarray((np.clip(edge, 0, 255) * 70 // 255).astype(np.uint8), "L"))
    body.alpha_composite(rim)
    body.putalpha(shape)
    out = Image.new("RGBA", (S, S), (0, 0, 0, 0))
    shadow = Image.new("L", (S, S), 0)
    shadow.paste(shape, ((S - B) // 2, (S - B) // 2 + 12))
    out.putalpha(shadow.filter(ImageFilter.GaussianBlur(14)).point(lambda v: v * 0.3))
    out.alpha_composite(body, ((S - B) // 2, (S - B) // 2))
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
    # The Mac: an iconset, which build-pkg.sh turns into the app's .icns (iconutil); up to 64 px, from the bold mark.
    tiles = {"mark": mac_tile(mark), "bold": mac_tile(Image.open(SMALL).convert("RGBA"))}
    os.makedirs("packaging/macos/offlok.iconset", exist_ok=True)
    for size in (16, 32, 128, 256, 512):
        for scale, suffix in ((1, ""), (2, "@2x")):
            px = size * scale
            tile = tiles["bold" if px <= 64 else "mark"]
            tile.resize((px, px), Image.LANCZOS).save(f"packaging/macos/offlok.iconset/icon_{size}x{size}{suffix}.png", optimize=True)
    # The website: the mark (hero, link previews), the bold mark where it is small (header, favicons), the tray's
    # states at twice their shown size, and a home-screen icon on the window's dark background.
    mark.resize((512, 512), Image.LANCZOS).save("site/img/offlok-mark.png", optimize=True)
    mark.resize((640, 640), Image.LANCZOS).save("site/img/offlok-mark.webp", quality=90, method=6)
    bold = Image.open(SMALL).convert("RGBA")
    bold.resize((64, 64), Image.LANCZOS).save("site/img/offlok-mark-small.png", optimize=True)
    icons = [padded(bold if n <= 32 else mark, n) for n in (48, 32, 16)]
    icons[0].save("site/favicon.ico", format="ICO", sizes=[(48, 48), (32, 32), (16, 16)], append_images=icons[1:])
    padded(mark, 192).save("site/img/icon-192.png", optimize=True)
    tile = Image.new("RGBA", (180, 180), (12, 19, 20, 255))
    tile.alpha_composite(padded(mark, 180, 0.78))
    tile.convert("RGB").save("site/img/apple-touch-icon.png", optimize=True)
    tray = bold.resize((384, 384), Image.LANCZOS)
    for name, img in {"on": tray, "fallback": badge(tray, ORANGE), "error": badge(tray, RED), "off": grey(tray)}.items():
        img.resize((96, 96), Image.LANCZOS).save(f"site/img/state-{name}.png", optimize=True)
    print("desktop/icons/tray-*.png, packaging/linux/icons/offlok-*.png, desktop/frontend/img/offlok-mark.png, packaging/windows/offlok.ico, packaging/macos/offlok.iconset/, site/")
