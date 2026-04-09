#!/usr/bin/env python3

from __future__ import annotations

import argparse
import re
import shutil
import subprocess
from pathlib import Path


def slugify(value: str) -> str:
    slug = re.sub(r"[^a-z0-9]+", "-", value.lower()).strip("-")
    return slug or "icon"


def render_template(template_path: Path, image_name: str, layer_name: str) -> str:
    template = template_path.read_text(encoding="utf-8")
    return (
        template.replace("__IMAGE_NAME__", image_name)
        .replace("__LAYER_NAME__", layer_name)
    )


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Create a clean Icon Composer .icon project from a source image."
    )
    parser.add_argument(
        "--source", required=True, help="Path to the cleaned source image."
    )
    parser.add_argument(
        "--output",
        required=True,
        help="Target .icon project path, for example assets/PrivateVPSPlatform.icon",
    )
    parser.add_argument(
        "--name",
        required=True,
        help="Human-readable app name used for the layer label.",
    )
    parser.add_argument(
        "--force",
        action="store_true",
        help="Overwrite the output project if it already exists.",
    )
    parser.add_argument(
        "--open",
        action="store_true",
        help="Open the generated project in Icon Composer and reveal it in Finder.",
    )
    args = parser.parse_args()

    source = Path(args.source).expanduser().resolve()
    output = Path(args.output).expanduser().resolve()
    template_path = (
        Path(__file__).resolve().parent.parent
        / "assets"
        / "icon-composer"
        / "icon.json.template"
    )

    if not source.is_file():
        raise SystemExit(f"Source image not found: {source}")

    if output.suffix != ".icon":
        raise SystemExit("Output path must end with .icon")

    if output.exists():
        if not args.force:
            raise SystemExit(
                f"Output already exists: {output}. Use --force to replace it."
            )
        shutil.rmtree(output)

    assets_dir = output / "Assets"
    assets_dir.mkdir(parents=True, exist_ok=True)

    file_slug = slugify(args.name)
    source_name = f"{file_slug}-icon-source{source.suffix.lower()}"
    target_image = assets_dir / source_name
    shutil.copy2(source, target_image)

    layer_name = f"{args.name} Source"
    icon_json = render_template(template_path, source_name, layer_name)
    (output / "icon.json").write_text(icon_json, encoding="utf-8")

    print(output)
    print(target_image)

    if args.open:
        subprocess.run(["open", str(output)], check=False)
        subprocess.run(["open", "-R", str(output)], check=False)

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
