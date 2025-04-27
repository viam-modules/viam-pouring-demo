package main

import (
	"context"
	"fmt"
	"image"
	"image/color"

	"github.com/golang/geo/r3"

	"go.viam.com/rdk/components/arm"
	"go.viam.com/rdk/components/camera"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/referenceframe"
	"go.viam.com/rdk/robot"
	"go.viam.com/rdk/services/motion"
	"go.viam.com/rdk/spatialmath"
)

func touchPrep(ctx context.Context, myRobot robot.Robot, myMotion motion.Service, arm arm.Arm, cam camera.Camera, logger logging.Logger) error {

	start := referenceframe.NewPoseInFrame(
		"world",
		spatialmath.NewPose(
			r3.Vector{X: -360, Y: 190, Z: 480},
			&spatialmath.OrientationVectorDegrees{OZ: -1, Theta: 160},
		),
	)

	done, err := myMotion.Move(
		ctx,
		motion.MoveReq{
			ComponentName: arm.Name(),
			Destination:   start,
		},
	)
	if err != nil {
		return err
	}
	if !done {
		return fmt.Errorf("first move didn't finish")
	}

	return nil
}

func touch(ctx context.Context, myRobot robot.Robot, myMotion motion.Service, arm arm.Arm, cam camera.Camera, logger logging.Logger) error {

	err := touchPrep(ctx, myRobot, myMotion, arm, cam, logger)
	if err != nil {
		return err
	}

	imgs, _, err := cam.Images(ctx)
	if err != nil {
		return err
	}

	if len(imgs) != 2 {
		return fmt.Errorf("expecting 2 images, got %d", len(imgs))
	}
	if imgs[1].SourceName != "depth" {
		return fmt.Errorf("img 1 name was %s, not depth", imgs[1].SourceName)
	}

	closest, distance := findClosestPoint(imgs[1].Image, centerPlus(imgs[1].Image, 40))
	logger.Infof("closest: %v distance: %v", closest, distance)

	touchPointRaw, err := camToWorld(ctx, myRobot, cam, closest, distance)
	if err != nil {
		return err
	}

	logger.Infof("touchPointRaw: %v", touchPointRaw)
	touchPoint := referenceframe.NewPoseInFrame(
		"world",
		spatialmath.NewPose(
			r3.Vector{X: touchPointRaw.Pose().Point().X, Y: touchPointRaw.Pose().Point().Y, Z: touchPointRaw.Pose().Point().Z + 200},
			&spatialmath.OrientationVectorDegrees{OZ: -1, Theta: 160},
		),
	)

	logger.Infof("touchPoint   : %v", touchPoint)

	done, err := myMotion.Move(
		ctx,
		motion.MoveReq{
			ComponentName: arm.Name(),
			Destination:   touchPoint,
		},
	)
	if err != nil {
		return err
	}
	if !done {
		return fmt.Errorf("first move didn't finish")
	}

	return nil
}

func camToWorld(ctx context.Context, myRobot robot.Robot, cam camera.Camera, pt image.Point, distance int) (*referenceframe.PoseInFrame, error) {
	properties, err := cam.Properties(ctx)
	if err != nil {
		return nil, err
	}

	x, y, z := properties.IntrinsicParams.PixelToPoint(float64(pt.X), float64(pt.X), float64(distance))
	p := spatialmath.NewPoseFromPoint(r3.Vector{X: x, Y: y, Z: z})
	return myRobot.TransformPose(ctx, referenceframe.NewPoseInFrame(cam.Name().ShortName(), p), "world", nil)
}

func centerPlus(i image.Image, extra int) image.Rectangle {
	centerX := (i.Bounds().Min.X + i.Bounds().Max.X) / 2
	centerY := (i.Bounds().Min.Y + i.Bounds().Max.Y) / 2

	return image.Rect(centerX-extra, centerY-extra, centerX+extra, centerY+extra)
}

func findClosestPoint(img image.Image, b image.Rectangle) (image.Point, int) {
	closest := 10000
	closestPoint := image.Point{}

	for x := b.Min.X; x < b.Max.X; x++ {
		for y := b.Min.Y; y < b.Max.Y; y++ {
			z := int((img.At(x, y).(color.Gray16)).Y)
			if z == 0 {
				continue
			}
			if z < closest {
				closest = z
				closestPoint.X = x
				closestPoint.Y = y
			}
		}
	}

	return closestPoint, closest
}
