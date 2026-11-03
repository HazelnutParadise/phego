<?php
declare(strict_types=1);
function w13R10StrictInstanceStore($object, string $name, mixed $value): void { $object->$name = $value; }
function w13R10StrictStaticStore(string $class, string $name, mixed $value): void { $class::$$name = $value; }
function w13R10StrictRawStore($object, string $name, mixed $value): void { (new ReflectionProperty($object, $name))->setRawValueWithoutLazyInitialization($object, $value); }
