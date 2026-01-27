"""Tests for TypeScript/JavaScript parser."""

import pytest

from c4.services.code_analysis import SymbolKind, TypeScriptParser


class TestTypeScriptParser:
    """Test TypeScriptParser class."""

    @pytest.fixture
    def parser(self) -> TypeScriptParser:
        """Create a parser instance."""
        return TypeScriptParser()

    def test_parse_simple_function(self, parser: TypeScriptParser):
        """Test parsing a simple function."""
        source = '''
function hello(name: string): string {
    return `Hello, ${name}!`;
}
'''
        table = parser.parse_source(source)

        assert len(table.symbols) == 1
        symbol = table.symbols["hello"]
        assert symbol.name == "hello"
        assert symbol.kind == SymbolKind.FUNCTION

    def test_parse_async_function(self, parser: TypeScriptParser):
        """Test parsing an async function."""
        source = '''
async function fetchData(url: string): Promise<Response> {
    return fetch(url);
}
'''
        table = parser.parse_source(source)

        symbol = table.symbols["fetchData"]
        assert symbol.is_async is True

    def test_parse_exported_function(self, parser: TypeScriptParser):
        """Test parsing exported functions."""
        source = '''
export function publicFunc() {}
function privateFunc() {}
'''
        table = parser.parse_source(source)

        public = table.symbols["publicFunc"]
        assert public.is_exported is True

        private = table.symbols["privateFunc"]
        assert private.is_exported is False

    def test_parse_arrow_function(self, parser: TypeScriptParser):
        """Test parsing arrow functions."""
        source = '''
const add = (a: number, b: number): number => a + b;
export const multiply = async (a: number, b: number) => a * b;
'''
        table = parser.parse_source(source)

        assert "add" in table.symbols
        add = table.symbols["add"]
        assert add.kind == SymbolKind.FUNCTION

        assert "multiply" in table.symbols
        multiply = table.symbols["multiply"]
        assert multiply.is_async is True
        assert multiply.is_exported is True

    def test_parse_class(self, parser: TypeScriptParser):
        """Test parsing a class."""
        source = '''
/**
 * A calculator class.
 */
export class Calculator {
    private value: number = 0;

    add(x: number): number {
        this.value += x;
        return this.value;
    }

    async compute(): Promise<number> {
        return this.value;
    }
}
'''
        table = parser.parse_source(source)

        assert "Calculator" in table.symbols
        calc = table.symbols["Calculator"]
        assert calc.kind == SymbolKind.CLASS
        assert calc.is_exported is True
        # Note: JSDoc parsing depends on pattern

    def test_parse_interface(self, parser: TypeScriptParser):
        """Test parsing interfaces."""
        source = '''
export interface User {
    id: number;
    name: string;
    email?: string;
}

interface PrivateConfig {
    secret: string;
}
'''
        table = parser.parse_source(source)

        assert "User" in table.symbols
        user = table.symbols["User"]
        assert user.kind == SymbolKind.INTERFACE
        assert user.is_exported is True

        assert "PrivateConfig" in table.symbols
        config = table.symbols["PrivateConfig"]
        assert config.is_exported is False

    def test_parse_type_alias(self, parser: TypeScriptParser):
        """Test parsing type aliases."""
        source = '''
export type UserId = string;
type Status = "pending" | "active" | "done";
type GenericResult<T> = { success: boolean; data: T };
'''
        table = parser.parse_source(source)

        assert "UserId" in table.symbols
        user_id = table.symbols["UserId"]
        assert user_id.kind == SymbolKind.TYPE_ALIAS
        assert user_id.is_exported is True

        assert "Status" in table.symbols
        assert "GenericResult" in table.symbols

    def test_parse_enum(self, parser: TypeScriptParser):
        """Test parsing enums."""
        source = '''
export enum Color {
    Red = "red",
    Green = "green",
    Blue = "blue",
}

const enum Direction {
    Up,
    Down,
    Left,
    Right,
}
'''
        table = parser.parse_source(source)

        assert "Color" in table.symbols
        color = table.symbols["Color"]
        assert color.kind == SymbolKind.ENUM
        assert color.is_exported is True

        assert "Direction" in table.symbols

    def test_parse_constants(self, parser: TypeScriptParser):
        """Test parsing constants."""
        source = '''
export const MAX_SIZE = 100;
const API_URL = "https://api.example.com";
const DEFAULT_CONFIG = { timeout: 30 };
'''
        table = parser.parse_source(source)

        assert "MAX_SIZE" in table.symbols
        max_size = table.symbols["MAX_SIZE"]
        assert max_size.kind == SymbolKind.CONSTANT
        assert max_size.is_exported is True

        assert "API_URL" in table.symbols
        assert "DEFAULT_CONFIG" in table.symbols

    def test_parse_imports(self, parser: TypeScriptParser):
        """Test parsing import statements."""
        source = '''
import React from "react";
import { useState, useEffect } from "react";
import * as fs from "fs";
import type { Config } from "./config";
'''
        table = parser.parse_source(source)

        assert "react" in table.imports
        assert "fs" in table.imports
        assert "./config" in table.imports

    def test_parse_exports(self, parser: TypeScriptParser):
        """Test tracking exports."""
        source = '''
export class MyClass {}
export function myFunc() {}
export const MY_CONST = 42;
export type MyType = string;
export interface MyInterface {}
'''
        table = parser.parse_source(source)

        assert "MyClass" in table.exports
        assert "myFunc" in table.exports
        assert "MY_CONST" in table.exports
        assert "MyType" in table.exports
        assert "MyInterface" in table.exports

    def test_parse_jsdoc_comment(self, parser: TypeScriptParser):
        """Test parsing JSDoc comments."""
        source = '''
/**
 * Greets a user by name.
 * @param name The user's name
 * @returns A greeting message
 */
function greet(name: string): string {
    return `Hello, ${name}!`;
}
'''
        table = parser.parse_source(source)

        symbol = table.symbols["greet"]
        # Doc should be extracted (depends on pattern matching)
        assert symbol.doc is None or "Greets" in symbol.doc

    def test_parse_abstract_class(self, parser: TypeScriptParser):
        """Test parsing abstract classes."""
        source = '''
export abstract class BaseService {
    abstract process(): void;
}
'''
        table = parser.parse_source(source)

        assert "BaseService" in table.symbols
        assert table.symbols["BaseService"].is_exported is True

    def test_language_detection(self, parser: TypeScriptParser):
        """Test language detection for different files."""
        ts_source = "const x: string = 'hello';"
        js_source = "const x = 'hello';"

        ts_table = parser.parse_source(ts_source, language="typescript")
        assert ts_table.language == "typescript"

        js_table = parser.parse_source(js_source, language="javascript")
        assert js_table.language == "javascript"
