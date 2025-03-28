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

The following attributes must be specified:

| Name                       | Type    | Inclusion    | Description                                                                                            |
| -------------------------- | ------- | ------------ | ------------------------------------------------------------------------------------------------------ |
| `arm_name`                 | string  | **Required** | The name of the arm which will do the pouring.                                                         |
| `camera_name`              | string  | **Required** | The name of the camera which will take images of the table.                                            |
| `circle_detection_service` | string  | **Required** | The name of the vision service which detects the number of cups and their positions.                   |
| `weight_sensor_name`       | string  | **Required** | The name of weight sensor which is used to determine how heavy the bottle is.                          |
| `address`                  | string  | **Required** | The address of the robot.                                                                              |
| `entity`                   | string  | **Required** | The entity of the robot.                                                                               |
| `payload`                  | string  | **Required** | The payload of the robot.                                                                              |
| `delta_x_pos`              | float64 | **Required** | A skew parameter used to adjust the postions of cups when translating from pixel space into the world. |
| `delta_y_pos`              | float64 | **Required** | A skew parameter used to adjust the postions of cups when translating from pixel space into the world. |
| `delta_x_neg`              | float64 | **Required** | A skew parameter used to adjust the postions of cups when translating from pixel space into the world. |
| `delta_y_neg`              | float64 | **Required** | A skew parameter used to adjust the postions of cups when translating from pixel space into the world. |
| `cpu_threads`              | int     | Optional     | Number of threads to use for planning. Half the availible threads will be used if unsupplied.

## What to do if something goes wrong

Use a pen or pencil to draw a circle around where your cup(s) were placed.
Message #team-motion on slack or reach out to Miko.
