"""#338 collision classes: property accessors, overload stubs, redefinition."""
from typing import overload
import typing

LIMIT: int
LIMIT = 10


@overload
def parse(x: int) -> int: ...
@overload
def parse(x: str) -> str: ...
def parse(x):
    return x


@typing.overload
def only_stubs(x: int) -> int: ...
@typing.overload
def only_stubs(x: str) -> str: ...


class Tokenizer:
    @property
    def model(self):
        return self._model

    @model.setter
    def model(self, value):
        self._model = value

    @model.deleter
    def model(self):
        del self._model

    def predict(self, features):
        return features

    def predict(self, features, threshold=0.5):  # noqa: F811 — later binding wins
        return features
