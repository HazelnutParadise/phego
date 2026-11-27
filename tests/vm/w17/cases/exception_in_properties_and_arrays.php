<?php
// exception_in_properties_and_arrays: exceptions held in properties, arrays, by-reference parameters and return values
class Holder {
    public $err;
    public $list = [];
}

function fill_ref(&$slot) {
    $slot = new Exception("by ref");
}

function give_back() {
    return new RuntimeException("returned");
}

$h = new Holder();
$h->err = new Exception("in property");
echo "property: ", $h->err->getMessage(), "\n";

$h->list[] = new Exception("in array");
$h->list[] = new LogicException("second in array");
echo "array_first: ", $h->list[0]->getMessage(), "\n";
$second = $h->list[1];
echo "array_second_class: ", $second::class, "\n";

$arr = [new Error("in literal"), new Exception("literal two")];
$lit0 = $arr[0];
echo "literal_first_class: ", $lit0::class, "\n";
echo "literal_second: ", $arr[1]->getMessage(), "\n";

$slot = null;
fill_ref($slot);
echo "ref_message: ", $slot->getMessage(), "\n";

$ret = give_back();
echo "return_message: ", $ret->getMessage(), "\n";
echo "return_class: ", $ret::class, "\n";

$nested = ['a' => ['b' => new Exception("deep")]];
echo "nested_message: ", $nested['a']['b']->getMessage(), "\n";

$h->err = null;
echo "property_cleared: ", ($h->err === null ? 1 : 0), "\n";
echo "array_still: ", $h->list[1]->getMessage(), "\n";
echo "array_first_still: ", ($h->list[0] instanceof Exception ? 1 : 0), "\n";
