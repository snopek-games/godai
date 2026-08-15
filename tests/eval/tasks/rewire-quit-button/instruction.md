In `res://menu.tscn`, clicking the QuitButton starts the game - its `pressed`
signal is wired to the wrong handler. It should call `_on_quit_button_pressed`
on the menu root instead. Fix the wiring and save the scene.
