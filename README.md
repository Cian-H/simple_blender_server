# Simple Blender render server
This is an extremely simple little blender render server. It accepts a POST request
on port 1212 at the `create_model` endpoint. A request to this endpoint returns a
binary blob of the GLB file for the model produced by that code. The JSON paylod
for the request must contain the key "model_code" with a string value containing a
string of python code for rendering a model.

The code given must contain a function with the signature `model() -> bpy.types.Object`.

# Example usage

As an example, this container can be used to create a simple model of a cube using the
following command:

```sh
curl -X POST http://localhost:1212/create_model -H "Content-Type: application/json" -d '{"model_code":"def model(): bpy.ops.mesh.primitive_cube_add(size=2, enter_editmode=False, align='WORLD', location=(0, 0, 0)); return bpy.context.object"}' --output model.glb
```
