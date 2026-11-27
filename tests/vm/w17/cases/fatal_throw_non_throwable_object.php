<?php
// fatal_throw_non_throwable_object: uncaught throw of a stdClass object that does not implement Throwable
$plain = new stdClass();
$plain->label = "not an exception";
$plain->count = 2;
echo "label: ", $plain->label, "\n";
echo "count: ", $plain->count, "\n";
echo "class: ", $plain::class, "\n";
echo "is_throwable: ", $plain instanceof Throwable ? "yes" : "no", "\n";
echo "is_stdclass: ", $plain instanceof stdClass ? "yes" : "no", "\n";
$copy = $plain;
echo "same_object: ", $copy === $plain ? "yes" : "no", "\n";
echo "copy_count: ", $copy->count, "\n";
echo "before\n";
throw $plain;
echo "never\n";
