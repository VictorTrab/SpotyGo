import math
from PIL import Image, ImageDraw, ImageFont

# Logo definition
banner = [
    "███████╗██████╗  ██████╗ ████████╗██╗███████╗██╗   ██╗ ██████╗  ██████╗ ",
    "██╔════╝██╔══██╗██╔═══██╗╚══██╔══╝██║██╔════╝╚██╗ ██╔╝██╔════╝ ██╔═══██╗",
    "███████╗██████╔╝██║   ██║   ██║   ██║█████╗   ╚████╔╝ ██║  ███╗██║   ██║",
    "╚════██║██╔═══╝ ██║   ██║   ██║   ██║██╔══╝    ╚██╔╝  ██║   ██║██║   ██║",
    "███████║██║     ╚██████╔╝   ██║   ██║██║        ██║   ╚██████╔╝╚██████╔╝",
    "╚══════╝╚═╝      ╚═════╝    ╚═╝   ╚═╝╚═╝        ╚═╝    ╚═════╝  ╚═════╝ ",
]

splitX = 56
logoW = 74
accentHex = "#1db954"
secondaryHex = "#06b6d4"
dimHex = "#1e293b"
glowHex = "#ffffff"
trailingHex = "#86efac"
leadingHex = "#15803d"

def hex_to_rgb(h):
    h = h.lstrip("#")
    return tuple(int(h[i:i+2], 16) for i in (0, 2, 4))

def lerp_color(c1, c2, t):
    t = max(0.0, min(1.0, t))
    return tuple(int(c1[i] * (1 - t) + c2[i] * t) for i in range(3))

accentRGB = hex_to_rgb(accentHex)
secondaryRGB = hex_to_rgb(secondaryHex)
dimRGB = hex_to_rgb(dimHex)
glowRGB = hex_to_rgb(glowHex)
trailingRGB = hex_to_rgb(trailingHex)
leadingRGB = hex_to_rgb(leadingHex)
bgRGB = (12, 13, 14)

font_size = 14
try:
    font = ImageFont.truetype("C:\\Windows\\Fonts\\consola.ttf", font_size)
except Exception:
    font = ImageFont.load_default()

# Measure char size using a block
bbox = font.getbbox("█")
char_w = bbox[2] - bbox[0] + 1
char_h = bbox[3] - bbox[1] + 3

padding_x = 40
padding_y = 35

img_w = padding_x * 2 + logoW * char_w
img_h = padding_y * 2 + len(banner) * char_h + 30

frames = []
total_frames = 65

for f in range(0, total_frames, 2):
    img = Image.new("RGB", (img_w, img_h), bgRGB)
    draw = ImageDraw.Draw(img)

    # Subtle terminal dots at top left
    draw.ellipse((16, 14, 24, 22), fill=(239, 68, 68))
    draw.ellipse((30, 14, 38, 22), fill=(234, 179, 8))
    draw.ellipse((44, 14, 52, 22), fill=(34, 197, 94))
    
    # Subtitle centered at top
    sub_title = "SpotifyGo · Terminal Hi-Fi Audio"
    sub_bbox = font.getbbox(sub_title)
    sub_w = sub_bbox[2] - sub_bbox[0]
    draw.text(((img_w - sub_w) // 2, 12), sub_title, font=font, fill=(100, 116, 139))

    sweepFrames = 45
    for row_idx, line in enumerate(banner):
        y = padding_y + 15 + row_idx * char_h
        for x, ch in enumerate(line):
            if ch == " ":
                continue
            
            if f <= sweepFrames:
                beamProgress = f / float(sweepFrames)
                beamX = int(beamProgress * (logoW + 16)) - 8
                dist = x - beamX
                if abs(dist) <= 1:
                    color = glowRGB
                elif -4 <= dist < 0:
                    color = trailingRGB
                elif dist < -4:
                    color = accentRGB if x < splitX else secondaryRGB
                elif 0 < dist <= 4:
                    color = leadingRGB
                else:
                    color = dimRGB
            else:
                base = accentRGB if x < splitX else secondaryRGB
                shimmerProgress = (f - sweepFrames) / float(total_frames - sweepFrames)
                wave = 0.5 + 0.5 * math.sin(2 * math.pi * (shimmerProgress * 2.0 - x / float(logoW)))
                color = lerp_color(base, glowRGB, 0.40 * wave)

            draw.text((padding_x + x * char_w, y), ch, font=font, fill=color)

    # Ambient bottom status
    status_text = "PRESS ANY KEY TO SKIP · LIBRESPOT ENGINE READY"
    st_bbox = font.getbbox(status_text)
    st_w = st_bbox[2] - st_bbox[0]
    draw.text(((img_w - st_w) // 2, img_h - 22), status_text, font=font, fill=(71, 85, 105))

    frames.append(img)

# Hold final frame a bit
for _ in range(8):
    frames.append(frames[-1])

frames[0].save(
    "assets/intro.gif",
    save_all=True,
    append_images=frames[1:],
    duration=50,
    loop=0,
    optimize=True
)
print("Saved assets/intro.gif successfully!")
