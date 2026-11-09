# Office pixel artwork

`generate.py` creates three connected isometric rooms, furniture layers, and character
sprites used by the browser office. No third-party artwork is bundled. Python 3
and Pillow are needed only to regenerate the artwork, not to build or run oak-tree.

From the repository root:

```sh
python3 tools/office-art/generate.py --export internal/oaktree/officeweb/assets
```

`scene.json` records the projection, room bounds and camera views, room-specific
seats/spots, furniture depth, collision rectangles, and sprite anchors. Work,
meeting, and break rooms share a connected floor with two partition doorways. The browser loads the small PNG assets once and animates their
positions on a Canvas; it does not fetch a new image for each frame.

For a standalone art preview, generate complete example frames outside the
checkout:

```sh
python3 tools/office-art/generate.py --out /tmp/oak-office-study --frames 32
```

These example frames use demonstration characters and labels. The browser office
instead uses real agent state and task/Campfire text from the authenticated local
API. The earlier terminal-graphics experiment is not part of the implementation.
