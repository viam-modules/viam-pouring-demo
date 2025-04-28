package main

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"math"

	"github.com/golang/geo/r3"

	"go.viam.com/rdk/components/arm"
	"go.viam.com/rdk/components/camera"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/pointcloud"
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

	touchPointRaw2d, err := findTouchPoint2d(ctx, myRobot, cam, logger)
	if err != nil {
		return err
	}

	if touchPointRaw2d.Parent() != "world" {
		return fmt.Errorf("frame wrong: %v", touchPointRaw2d.Parent())
	}

	logger.Infof("touchPointRaw: %v", touchPointRaw2d)

	touchPointRaw3d, err := findTouchPoint3d(ctx, myRobot, cam, logger)
	if err != nil {
		return err
	}

	if touchPointRaw3d.Parent() != "world" {
		return fmt.Errorf("frame wrong: %v", touchPointRaw3d.Parent())
	}

	logger.Infof("touchPointRaw3d: %v", touchPointRaw3d)

	touchPointRaw := touchPointRaw3d

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

func findTouchPoint3d(ctx context.Context, myRobot robot.Robot, cam camera.Camera, logger logging.Logger) (*referenceframe.PoseInFrame, error) {
	pc, err := cam.NextPointCloud(ctx)
	if err != nil {
		return nil, err
	}

	closest := r3.Vector{0, 0, 0}

	pc.Iterate(0, 0, func(p r3.Vector, d pointcloud.Data) bool {
		xydist := math.Pow((p.X*p.X)+(p.Y*p.Y), .5)
		if xydist > .0001 && xydist < 100 {
			if closest.Z == 0 || p.Z < closest.Z {
				closest = p
			}
		}
		return true
	})

	logger.Infof("closest in 3d cam: %v", closest)

	return myRobot.TransformPose(
		ctx,
		referenceframe.NewPoseInFrame(
			cam.Name().ShortName(),
			spatialmath.NewPoseFromPoint(closest),
		),
		"world",
		nil,
	)
}

func findTouchPoint2d(ctx context.Context, myRobot robot.Robot, cam camera.Camera, logger logging.Logger) (*referenceframe.PoseInFrame, error) {
	imgs, _, err := cam.Images(ctx)
	if err != nil {
		return nil, err
	}

	if len(imgs) != 2 {
		return nil, fmt.Errorf("expecting 2 images, got %d", len(imgs))
	}
	if imgs[1].SourceName != "depth" {
		return nil, fmt.Errorf("img 1 name was %s, not depth", imgs[1].SourceName)
	}

	closest, distance := findClosestPoint(imgs[1].Image, centerPlus(imgs[1].Image, 40))
	logger.Infof("closest: %v distance: %v", closest, distance)

	properties, err := cam.Properties(ctx)
	if err != nil {
		return nil, err
	}

	x, y, z := properties.IntrinsicParams.PixelToPoint(float64(closest.X), float64(closest.X), float64(distance))
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
