#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import subprocess
from dataclasses import dataclass
from pathlib import Path
from typing import Iterable


INTERIOR_DIRECTIONS: dict[str, tuple[str, ...]] = {
    "empty": (),
    "vertical": ("n", "s"),
    "horizontal": ("e", "w"),
    "cross": ("n", "e", "s", "w"),
    "tee-n": ("n", "e", "w"),
    "tee-e": ("n", "e", "s"),
    "tee-s": ("e", "s", "w"),
    "tee-w": ("n", "s", "w"),
    "turn-ne": ("n", "e"),
    "turn-nw": ("n", "w"),
    "turn-se": ("s", "e"),
    "turn-sw": ("s", "w"),
}

EDGE_OPPOSITE = {"n": "s", "e": "w", "s": "n", "w": "e"}
VALID_EDGES = set(EDGE_OPPOSITE)
VALID_KINDS = {"content", "pipe", "blank"}
VALID_LANES = {"upper", "center", "lower"}


@dataclass(frozen=True)
class Cell:
    row: int
    col: int
    span: int
    kind: str
    label: str
    edges: frozenset[str]
    interior: str

    def bounds(self, cell_width: int, cell_height: int) -> tuple[int, int, int, int]:
        x0 = self.col * (cell_width + 1)
        y0 = self.row * (cell_height + 1)
        x1 = x0 + self.span * (cell_width + 1)
        y1 = y0 + cell_height + 1
        return x0, y0, x1, y1


@dataclass(frozen=True)
class Diagram:
    title: str
    columns: int
    cell_width: int
    cell_height: int
    rows: tuple[tuple[Cell, ...], ...]
    row_lanes: tuple[str, ...]


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Render validated ASCII grid diagrams.")
    parser.add_argument("--spec", type=Path, required=True, help="Path to the JSON spec.")
    parser.add_argument("--format-spec-out", type=Path, help="Write a canonical formatted spec to this file.")
    parser.add_argument("--ascii-out", type=Path, help="Write ASCII output to this file.")
    parser.add_argument("--png-out", type=Path, help="Write PNG preview to this file.")
    parser.add_argument("--png-scale", type=int, default=3, help="PNG render scale factor, default 3.")
    parser.add_argument("--stdout", action="store_true", help="Print ASCII output to stdout.")
    return parser.parse_args()


def load_diagram(path: Path) -> Diagram:
    data = json.loads(path.read_text())

    title = str(data.get("title", "")).strip()
    columns = int(data["columns"])
    cell_width = int(data.get("cell_width", 5))
    cell_height = int(data.get("cell_height", 3))

    if columns <= 0:
      raise ValueError("columns must be positive.")
    if cell_width < 3 or cell_width % 2 == 0:
      raise ValueError("cell_width must be an odd integer >= 3.")
    if cell_height < 1:
      raise ValueError("cell_height must be >= 1.")

    rows: list[tuple[Cell, ...]] = []
    row_lanes: list[str] = []

    for row_index, row_data in enumerate(data["rows"]):
        row_cells: list[Cell] = []
        lane = str(row_data.get("lane", "center"))

        if lane not in VALID_LANES:
            raise ValueError(f"row {row_index}: invalid lane {lane!r}")

        for cell_data in row_data["cells"]:
            col = int(cell_data["col"])
            span = int(cell_data.get("span", 1))
            kind = str(cell_data.get("kind", "content"))
            label = str(cell_data.get("label", ""))
            edges = frozenset(str(edge) for edge in cell_data.get("edges", []))
            interior = str(cell_data.get("interior", "empty"))

            if kind not in VALID_KINDS:
                raise ValueError(f"row {row_index}: invalid kind {kind!r}")
            if interior not in INTERIOR_DIRECTIONS:
                raise ValueError(f"row {row_index}: invalid interior {interior!r}")
            if not edges.issubset(VALID_EDGES):
                raise ValueError(f"row {row_index}: invalid edges {sorted(edges)}")
            if span <= 0:
                raise ValueError(f"row {row_index}: span must be positive")

            row_cells.append(
                Cell(
                    row=row_index,
                    col=col,
                    span=span,
                    kind=kind,
                    label=label,
                    edges=edges,
                    interior=interior,
                )
            )

        validate_row(columns, row_index, row_cells)
        rows.append(tuple(sorted(row_cells, key=lambda cell: cell.col)))
        row_lanes.append(lane)

    diagram = Diagram(
        title=title,
        columns=columns,
        cell_width=cell_width,
        cell_height=cell_height,
        rows=tuple(rows),
        row_lanes=tuple(row_lanes),
    )
    validate_boundaries(diagram)
    return diagram


def validate_row(columns: int, row_index: int, cells: list[Cell]) -> None:
    if not cells:
        raise ValueError(f"row {row_index}: must declare at least one cell")

    expected_col = 0

    for cell in sorted(cells, key=lambda current: current.col):
        if cell.col != expected_col:
            raise ValueError(
                f"row {row_index}: expected cell at col {expected_col}, got {cell.col}"
            )

        expected_col += cell.span

    if expected_col != columns:
        raise ValueError(f"row {row_index}: cells cover {expected_col} columns, expected {columns}")


def validate_boundaries(diagram: Diagram) -> None:
    openings: dict[tuple[str, int, int], list[tuple[int, int, str]]] = {}
    max_x = diagram.columns * (diagram.cell_width + 1)
    max_y = len(diagram.rows) * (diagram.cell_height + 1)

    for row in diagram.rows:
        for cell in row:
            x0, y0, x1, y1 = cell.bounds(diagram.cell_width, diagram.cell_height)
            centers = {
                "n": ("h", y0, (x0 + x1) // 2),
                "s": ("h", y1, (x0 + x1) // 2),
                "w": ("v", x0, (y0 + y1) // 2),
                "e": ("v", x1, (y0 + y1) // 2),
            }

            for edge in cell.edges:
                orientation, line, center = centers[edge]
                if orientation == "h" and line in {0, max_y}:
                    raise ValueError(f"row {cell.row}, col {cell.col}: pipe opening {edge} hits outer boundary")
                if orientation == "v" and line in {0, max_x}:
                    raise ValueError(f"row {cell.row}, col {cell.col}: pipe opening {edge} hits outer boundary")

                openings.setdefault(centers[edge], []).append((cell.row, cell.col, edge))

    for key, values in openings.items():
        if len(values) != 2:
            raise ValueError(f"unmatched pipe crossing at {key}: {values}")

        left, right = values
        if EDGE_OPPOSITE[left[2]] != right[2]:
            raise ValueError(f"mismatched pipe crossing at {key}: {values}")


def render_ascii(diagram: Diagram) -> str:
    diagram = format_diagram(diagram)
    body = render_ascii_body(diagram)

    if diagram.title:
        return f"{diagram.title}\n\n{body}"

    return body


def render_ascii_body(diagram: Diagram) -> str:
    width = diagram.columns * (diagram.cell_width + 1) + 1
    height = len(diagram.rows) * (diagram.cell_height + 1) + 1
    canvas = [[" " for _ in range(width)] for _ in range(height)]

    for row_index, row in enumerate(diagram.rows):
        for cell in row:
            draw_cell(canvas, diagram, cell, diagram.row_lanes[row_index])

    return "\n".join("".join(line) for line in canvas)


def canonical_spec(diagram: Diagram) -> dict:
    diagram = format_diagram(diagram)
    rows: list[dict] = []

    for lane, row in zip(diagram.row_lanes, diagram.rows):
        row_payload: dict = {"lane": lane, "cells": []}

        for cell in row:
            payload: dict[str, object] = {"col": cell.col}

            if cell.span != 1:
                payload["span"] = cell.span
            if cell.kind != "content":
                payload["kind"] = cell.kind
            if cell.label:
                payload["label"] = cell.label
            if cell.edges:
                payload["edges"] = sorted(cell.edges)
            if cell.interior != "empty":
                payload["interior"] = cell.interior

            row_payload["cells"].append(payload)

        rows.append(row_payload)

    return {
        "title": diagram.title,
        "columns": diagram.columns,
        "cell_width": diagram.cell_width,
        "cell_height": diagram.cell_height,
        "rows": rows,
    }


def format_diagram(diagram: Diagram) -> Diagram:
    formatted_lanes: list[str] = []

    for lane, row in zip(diagram.row_lanes, diagram.rows):
        has_horizontal_pipe = any(
            ("e" in cell.edges or "w" in cell.edges or "e" in INTERIOR_DIRECTIONS[cell.interior] or "w" in INTERIOR_DIRECTIONS[cell.interior])
            for cell in row
        )
        formatted_lanes.append(lane if has_horizontal_pipe else "center")

    return Diagram(
        title=diagram.title,
        columns=diagram.columns,
        cell_width=diagram.cell_width,
        cell_height=diagram.cell_height,
        rows=diagram.rows,
        row_lanes=tuple(formatted_lanes),
    )


def resolve_lane_y(y0: int, y1: int, lane: str) -> int:
    if lane == "upper":
        return y0 + 1
    if lane == "lower":
        return y1 - 1
    return (y0 + y1) // 2


def draw_cell(canvas: list[list[str]], diagram: Diagram, cell: Cell, lane: str) -> None:
    x0, y0, x1, y1 = cell.bounds(diagram.cell_width, diagram.cell_height)

    put(canvas, x0, y0, "+")
    put(canvas, x1, y0, "+")
    put(canvas, x0, y1, "+")
    put(canvas, x1, y1, "+")

    for x in range(x0 + 1, x1):
        put(canvas, x, y0, "-")
        put(canvas, x, y1, "-")

    for y in range(y0 + 1, y1):
        put(canvas, x0, y, "|")
        put(canvas, x1, y, "|")

    lane_y = resolve_lane_y(y0, y1, lane)
    draw_edge_pipes(canvas, x0, y0, x1, y1, cell.edges, lane_y)
    draw_interior(canvas, x0, y0, x1, y1, cell.interior, lane_y)

    if cell.label:
        draw_centered_text(canvas, x0, y0, x1, y1, cell.label)


def draw_edge_pipes(
    canvas: list[list[str]], x0: int, y0: int, x1: int, y1: int, edges: Iterable[str], lane_y: int
) -> None:
    center_x = (x0 + x1) // 2

    for edge in edges:
        if edge == "n":
            put(canvas, center_x, y0, "|", override=True)
        elif edge == "s":
            put(canvas, center_x, y1, "|", override=True)
        elif edge == "w":
            put(canvas, x0, lane_y, "-", override=True)
        elif edge == "e":
            put(canvas, x1, lane_y, "-", override=True)


def draw_interior(
    canvas: list[list[str]], x0: int, y0: int, x1: int, y1: int, interior: str, lane_y: int
) -> None:
    directions = INTERIOR_DIRECTIONS[interior]

    if not directions:
        return

    center_x = (x0 + x1) // 2

    for direction in directions:
        if direction == "n":
            for y in range(y0 + 1, lane_y + 1):
                put_pipe(canvas, center_x, y, "|")
        elif direction == "s":
            for y in range(lane_y, y1):
                put_pipe(canvas, center_x, y, "|")
        elif direction == "w":
            for x in range(x0 + 1, center_x + 1):
                put_pipe(canvas, x, lane_y, "-")
        elif direction == "e":
            for x in range(center_x, x1):
                put_pipe(canvas, x, lane_y, "-")

    has_vertical = "n" in directions or "s" in directions
    has_horizontal = "e" in directions or "w" in directions

    if has_vertical and has_horizontal:
        put_pipe(canvas, center_x, lane_y, "+")
    elif has_vertical:
        put_pipe(canvas, center_x, lane_y, "|")
    elif has_horizontal:
        put_pipe(canvas, center_x, lane_y, "-")


def draw_centered_text(
    canvas: list[list[str]], x0: int, y0: int, x1: int, y1: int, label: str
) -> None:
    lines = label.splitlines()[: max(1, y1 - y0 - 1)]
    content_width = x1 - x0 - 1
    content_height = y1 - y0 - 1
    first_line_y = y0 + 1 + max(0, (content_height - len(lines)) // 2)

    for line_index, raw_line in enumerate(lines):
        text = raw_line[:content_width]
        start_x = x0 + 1 + max(0, (content_width - len(text)) // 2)
        y = first_line_y + line_index

        for offset, character in enumerate(text):
            put(canvas, start_x + offset, y, character, override=True)


def put(canvas: list[list[str]], x: int, y: int, character: str, override: bool = False) -> None:
    existing = canvas[y][x]

    if override or existing == " " or existing == character:
        canvas[y][x] = character
        return

    if {existing, character} <= {"+", "-", "|"}:
        canvas[y][x] = "+"
        return

    canvas[y][x] = character


def put_pipe(canvas: list[list[str]], x: int, y: int, character: str) -> None:
    existing = canvas[y][x]

    if existing in {" ", character}:
        canvas[y][x] = character
    elif {existing, character} <= {"-", "|", "+"}:
        canvas[y][x] = "+"
    else:
        canvas[y][x] = character


def load_monospace_font(size: int):
    from PIL import ImageFont

    candidates: list[str] = []

    try:
        result = subprocess.run(
            ["fc-match", "-f", "%{file}\n", "monospace"],
            check=False,
            capture_output=True,
            text=True,
        )
        font_path = result.stdout.strip()
        if font_path:
            candidates.append(font_path)
    except OSError:
        pass

    candidates.extend(
        [
            "/System/Library/Fonts/Supplemental/Andale Mono.ttf",
            "/System/Library/Fonts/Menlo.ttc",
            "/Library/Fonts/Courier New.ttf",
            "Andale Mono.ttf",
            "Menlo.ttc",
            "Courier New.ttf",
            "DejaVuSansMono.ttf",
        ]
    )

    seen: set[str] = set()

    for candidate in candidates:
        if candidate in seen:
            continue
        seen.add(candidate)
        try:
            return ImageFont.truetype(candidate, size)
        except OSError:
            continue

    return ImageFont.load_default()


def split_title_and_body(text: str) -> tuple[str, list[str]]:
    lines = text.splitlines()

    if len(lines) >= 3 and lines[1] == "":
        return lines[0], lines[2:]

    return "", lines


def render_png(text: str, output_path: Path, scale: int) -> None:
    try:
        from PIL import Image, ImageDraw
    except ImportError as exc:  # pragma: no cover
        raise RuntimeError("Pillow is required for --png-out.") from exc

    if scale <= 0:
        raise ValueError("png_scale must be positive.")

    title, body_lines = split_title_and_body(text)
    body_lines = body_lines or [""]

    title_font = load_monospace_font(22 * scale)
    glyph_font = load_monospace_font(20 * scale)
    cell_width = 20 * scale
    cell_height = 28 * scale
    padding_x = 36 * scale
    padding_y = 28 * scale
    title_gap = 26 * scale if title else 0
    stroke_width = max(2, 2 * scale)
    line_margin_x = max(4, 4 * scale)
    line_margin_y = max(4, 4 * scale)

    title_bbox = title_font.getbbox(title or "M")
    title_width = (title_bbox[2] - title_bbox[0]) if title else 0
    title_height = (title_bbox[3] - title_bbox[1]) if title else 0
    body_width_chars = max(len(line) for line in body_lines)
    body_height_rows = len(body_lines)

    image_width = max(body_width_chars * cell_width, title_width) + padding_x * 2
    image_height = (
        body_height_rows * cell_height
        + padding_y * 2
        + title_height
        + title_gap
    )

    image = Image.new("RGB", (image_width, image_height), "#ffffff")
    draw = ImageDraw.Draw(image)

    if title:
        draw.text((padding_x, padding_y), title, fill="#111111", font=title_font)

    origin_y = padding_y + title_height + title_gap

    for row_index, line in enumerate(body_lines):
        for col_index, character in enumerate(line):
            x = padding_x + col_index * cell_width
            y = origin_y + row_index * cell_height
            center_x = x + cell_width // 2
            center_y = y + cell_height // 2

            if character == "-":
                draw.line(
                    [(x + line_margin_x, center_y), (x + cell_width - line_margin_x, center_y)],
                    fill="#111111",
                    width=stroke_width,
                )
            elif character == "|":
                draw.line(
                    [(center_x, y + line_margin_y), (center_x, y + cell_height - line_margin_y)],
                    fill="#111111",
                    width=stroke_width,
                )
            elif character == "+":
                draw.line(
                    [(x + line_margin_x, center_y), (x + cell_width - line_margin_x, center_y)],
                    fill="#111111",
                    width=stroke_width,
                )
                draw.line(
                    [(center_x, y + line_margin_y), (center_x, y + cell_height - line_margin_y)],
                    fill="#111111",
                    width=stroke_width,
                )
            elif character != " ":
                bbox = glyph_font.getbbox(character)
                text_width = bbox[2] - bbox[0]
                text_height = bbox[3] - bbox[1]
                text_x = x + (cell_width - text_width) / 2
                text_y = y + (cell_height - text_height) / 2 - bbox[1]
                draw.text((text_x, text_y), character, fill="#111111", font=glyph_font)

    image.save(output_path)


def main() -> None:
    args = parse_args()
    diagram = load_diagram(args.spec)
    ascii_output = render_ascii(diagram)

    if args.format_spec_out:
        args.format_spec_out.parent.mkdir(parents=True, exist_ok=True)
        args.format_spec_out.write_text(f"{json.dumps(canonical_spec(diagram), indent=2)}\n")

    if args.stdout or not args.ascii_out and not args.png_out:
        print(ascii_output)

    if args.ascii_out:
        args.ascii_out.parent.mkdir(parents=True, exist_ok=True)
        args.ascii_out.write_text(ascii_output)

    if args.png_out:
        args.png_out.parent.mkdir(parents=True, exist_ok=True)
        render_png(ascii_output, args.png_out, args.png_scale)


if __name__ == "__main__":
    main()
