# viam-pouring-demo

This is a module for the pouring demo.
The demo required three pieces: an arm, a weight sensor, and a camera.
The camera is used to take pictures of table, identifying how many cups there are as well as their positions.
The weight sensor is used to inform the demo about how heavy the bottle is. Depending on how heavy the bottle is, we know how full it is and therefore what angle to hold the bottle at when dispensing liquid and for how long.

## How to run the demo

A user is required to start a server for the arm. The arm may be found in viam-dev/motion-team/waiter. Once the server is started the user should go to app.viam/control and find "pouring-service". To start the demo simply click execute on the DoCommand passing in an empty argument.

## How the demo works

First we take a picture of the table to determine how many cups there are.
Then, we take 20 images of the table, leveraging big data, to determine their positions.
We take the average of positions found across the 20 images to get the average of the positions of cups.
Then, we pre-plan all motions the arm will have to execute.
Once planning is done the demo executes - liquid is dispensed into the cups.

## Attributes

The `viam:pouring-demo:vinocart` service takes the following attributes:

| Name                          | Type    | Inclusion              | Description                                                                                                          |
| ----------------------------- | ------- | ---------------------- | ------------------------------------------------------------------------------------------------------------------- |
| `arm_name`                    | string  | **Required**           | The arm that picks and places the cup.                                                                              |
| `gripper_name`                | string  | **Required**           | The gripper on the cup arm.                                                                                          |
| `camera_name`                 | string  | **Required**           | The camera used to capture pick-quality images of the grabbed cup.                                                  |
| `bottle_height`               | float64 | **Required**           | Height of the bottle (mm); used to locate the bottle tip (`bottle-top` frame) for pouring.                          |
| `cup_cloud_cam`               | string  | Required for picking   | Camera that returns a single point cloud of just the cup. The cup is found and measured directly from this cloud.   |
| `bottle_arm`                  | string  | Required for pouring   | The arm that holds and tilts the bottle.                                                                            |
| `bottle_gripper`              | string  | Required for pouring   | The gripper on the bottle arm.                                                                                      |
| `Positions`                   | object  | Required for motion    | Named stage/step joint positions (toggleswitch components) the arms replay. Note: capitalized key, no JSON tag.     |
| `glass_pour_cam`              | string  | Optional               | Camera that watches the glass fill during a pour.                                                                   |
| `glass_pour_motion_threshold` | float64 | Optional               | Grayscale-delta threshold for detecting liquid hitting the glass (default 4).                                       |
| `pour_glass_find_service`     | string  | Optional               | Vision service that locates and crops the glass for pour monitoring.                                                |
| `glass_fullness_service`      | string  | Optional               | Vision (classification) service for ML-based glass-fullness detection.                                              |
| `use_glass_fullness_model`    | bool    | Optional               | Use the ML fullness model instead of the default image-delta motion detection.                                      |
| `pick_quality_service`        | string  | Optional               | Vision (classification) service that grades whether a cup pick was good.                                            |
| `cup_grip_height_offset`      | float64 | Optional               | Distance (mm) below the cup rim to grip/release the cup (default 25).                                               |
| `cup_top_offset_x`/`_y`/`_z`  | float64 | Optional               | Offsets (mm) of the `cup-top` frame relative to the gripper (defaults: X = `cup_grip_height_offset`, Y = -75, Z = -25). |
| `cup`                         | object  | Optional               | "Standardized cup" validation spec a detected cup is checked against. See below.                                    |
| `Handoff`                     | bool    | Optional               | Allow handing the cup from the bottle arm to the cup arm when the cup arm can't reach it. Capitalized key, no JSON tag. |
| `loop`                        | bool    | Optional               | Run the demo continuously: find cup → pour → wait for the area to clear → repeat.                                   |

### `cup` (standardized-cup validation)

After the cup point cloud is measured (center, height, width, point count), it is validated against this spec. Every field is optional with a sensible default; omit the whole `cup` block to use all defaults.

| Name                  | Type    | Inclusion | Description                                                                                              |
| --------------------- | ------- | --------- | ------------------------------------------------------------------------------------------------------- |
| `min_height`          | float64 | Optional  | Minimum valid cup height in mm (default 30).                                                             |
| `max_height`          | float64 | Optional  | Maximum valid cup height in mm (default 400).                                                            |
| `min_width`           | float64 | Optional  | Minimum valid cup width in mm (default 20).                                                              |
| `max_width`           | float64 | Optional  | Maximum valid cup width in mm (default 200).                                                             |
| `gripper_max_opening` | float64 | Optional  | Gripper's maximum span in mm. When > 0, rejects cups too wide for the gripper to wrap around. Disabled (0) by default. |
| `grip_clearance`      | float64 | Optional  | Margin (mm) beyond the cup width the gripper needs to wrap; also inflates the cup collision obstacle (default 10). |
| `min_points`          | int     | Optional  | Minimum number of cloud points required to trust a detection — guards against sparse/see-through clouds (default 20). |
| `nominal_height`      | float64 | Optional  | Fallback cup height (mm) used to derive a release height before the first cup is picked this run (default 120). |

## What to do if something goes wrong

Use a pen or pencil to draw a circle around where your cup(s) were placed.
Message #team-motion on slack or reach out to Miko.
