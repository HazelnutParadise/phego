<?php
// closure_names: magic constants inside closures
$top = function () {
    echo "top_fn: ", __FUNCTION__, "\n";
    echo "top_method: ", __METHOD__, "\n";
    echo "top_class: ", __CLASS__, "\n";
};
$top();

function maker() {
    $inner = function () {
        echo "in_func_fn: ", __FUNCTION__, "\n";
        echo "in_func_method: ", __METHOD__, "\n";
    };
    $inner();
    $arrow = fn() => __FUNCTION__;
    echo "in_func_arrow: ", $arrow(), "\n";
}
maker();

class K {
    public function run() {
        $c = function () {
            echo "method_closure_fn: ", __FUNCTION__, "\n";
            echo "method_closure_method: ", __METHOD__, "\n";
            echo "method_closure_class: ", __CLASS__, "\n";
        };
        $c();
        $d = function () {
            $e = function () {
                return __FUNCTION__;
            };
            return $e();
        };
        echo "nested_closure_fn: ", $d(), "\n";
        $ar = fn() => __METHOD__;
        echo "method_arrow_method: ", $ar(), "\n";
    }
}
(new K())->run();
echo "arrow_top: ", (fn() => __FUNCTION__)(), "\n";
echo "static_top: ", (static function () { return __FUNCTION__; })(), "\n";
